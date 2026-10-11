// Package socks5 implements the minimal SOCKS5 server (RFC 1928) the gopskit bridge runs inside a
// Kubernetes cluster.
//
// It supports exactly what the gopskit CLIs need and nothing else: the CONNECT command, without
// authentication, to IPv4, IPv6 and domain-name destinations. Domain names are resolved by the
// server, which is the point of the bridge - a CLI outside the cluster asks for
// "keycloak.keycloak.svc:8443" and the cluster's own DNS answers it.
//
// The server performs no authentication of its own. The bridge listens on the pod's loopback
// interface only, so the sole way to reach it is a Kubernetes port-forward, and permission to
// port-forward to the bridge pod is the access control.
package socks5

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/adnoctem/gopskit/pkg/log"
)

// Protocol constants from RFC 1928.
const (
	version5 = 0x05

	methodNoAuth       = 0x00
	methodNoAcceptable = 0xFF

	cmdConnect = 0x01

	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04
)

// Reply codes from RFC 1928, section 6.
const (
	ReplySucceeded           byte = 0x00
	ReplyGeneralFailure      byte = 0x01
	ReplyNotAllowed          byte = 0x02
	ReplyNetworkUnreachable  byte = 0x03
	ReplyHostUnreachable     byte = 0x04
	ReplyConnectionRefused   byte = 0x05
	ReplyTTLExpired          byte = 0x06
	ReplyCommandNotSupported byte = 0x07
	ReplyAddressNotSupported byte = 0x08
)

const (
	// DefaultHandshakeTimeout bounds the negotiation phase, so a client that connects and then
	// says nothing cannot hold a connection open indefinitely.
	DefaultHandshakeTimeout = 10 * time.Second

	// DefaultDialTimeout bounds the outbound connection to the requested destination.
	DefaultDialTimeout = 15 * time.Second
)

// ErrServerClosed is returned by Serve after Shutdown or after its context is cancelled.
var ErrServerClosed = errors.New("socks5: server closed")

// Server is a SOCKS5 server. The zero value is usable; every field is optional.
type Server struct {
	// Dialer opens the outbound connection. It defaults to a net.Dialer with DefaultDialTimeout.
	Dialer interface {
		DialContext(ctx context.Context, network, address string) (net.Conn, error)
	}

	// Allow, when set, is consulted before every outbound connection. Returning false refuses
	// the request with ReplyNotAllowed. host is the destination exactly as the client sent it,
	// a domain name or an IP literal.
	Allow func(host string, port int) bool

	// HandshakeTimeout overrides DefaultHandshakeTimeout.
	HandshakeTimeout time.Duration

	// Log receives one line per connection and any unexpected errors. Nil disables logging.
	Log *log.Logger

	mu        sync.Mutex
	listeners map[net.Listener]struct{}
	conns     map[net.Conn]struct{}
	closed    bool
	wg        sync.WaitGroup
}

// Serve accepts connections on l until ctx is cancelled or Shutdown is called, and always
// returns a non-nil error: ErrServerClosed on an orderly stop. Serve may be called concurrently
// on several listeners, e.g. one for 127.0.0.1 and one for ::1.
func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	if !s.track(l) {
		_ = l.Close()
		return ErrServerClosed
	}
	defer s.untrack(l)

	stop := context.AfterFunc(ctx, func() { _ = s.Shutdown() })
	defer stop()

	for {
		conn, err := l.Accept()
		if err != nil {
			if s.isClosed() {
				return ErrServerClosed
			}

			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}

			return err
		}

		if !s.trackConn(conn) {
			_ = conn.Close()
			return ErrServerClosed
		}

		s.wg.Go(func() {
			defer s.untrackConn(conn)

			s.handle(ctx, conn)
		})
	}
}

// Shutdown closes every listener and every open connection, then waits for the connection
// handlers to return. It is safe to call more than once.
func (s *Server) Shutdown() error {
	s.mu.Lock()
	s.closed = true
	for l := range s.listeners {
		_ = l.Close()
	}
	for c := range s.conns {
		_ = c.Close()
	}
	s.mu.Unlock()

	s.wg.Wait()
	return nil
}

func (s *Server) handle(ctx context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()

	timeout := s.HandshakeTimeout
	if timeout <= 0 {
		timeout = DefaultHandshakeTimeout
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))

	if err := negotiate(conn); err != nil {
		s.debugf("negotiation with %s failed: %v", conn.RemoteAddr(), err)
		return
	}

	host, port, reply, err := readRequest(conn)
	if err != nil {
		s.debugf("bad request from %s: %v", conn.RemoteAddr(), err)
		if reply != ReplySucceeded {
			_ = writeReply(conn, reply, nil)
		}
		return
	}

	if s.Allow != nil && !s.Allow(host, port) {
		s.infof("refused %s -> %s: not allowed", conn.RemoteAddr(), net.JoinHostPort(host, strconv.Itoa(port)))
		_ = writeReply(conn, ReplyNotAllowed, nil)
		return
	}

	target := net.JoinHostPort(host, strconv.Itoa(port))
	dialCtx, cancel := context.WithTimeout(ctx, DefaultDialTimeout)
	upstream, err := s.dialer().DialContext(dialCtx, "tcp", target)
	cancel()
	if err != nil {
		s.infof("connect %s -> %s failed: %v", conn.RemoteAddr(), target, err)
		_ = writeReply(conn, replyFor(err), nil)
		return
	}
	defer func() { _ = upstream.Close() }()

	if !s.trackConn(upstream) {
		return
	}
	defer s.untrackConn(upstream)

	if err := writeReply(conn, ReplySucceeded, upstream.LocalAddr()); err != nil {
		return
	}

	// the handshake is over; the tunnel lives as long as both sides want it to
	_ = conn.SetDeadline(time.Time{})
	s.infof("connect %s -> %s", conn.RemoteAddr(), target)

	relay(conn, upstream)
}

func (s *Server) dialer() interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
} {
	if s.Dialer != nil {
		return s.Dialer
	}

	return &net.Dialer{Timeout: DefaultDialTimeout}
}

// negotiate handles the method-selection message. Only "no authentication" is offered.
func negotiate(rw io.ReadWriter) error {
	var hdr [2]byte
	if _, err := io.ReadFull(rw, hdr[:]); err != nil {
		return err
	}

	if hdr[0] != version5 {
		return fmt.Errorf("unsupported SOCKS version %d", hdr[0])
	}

	methods := make([]byte, hdr[1])
	if _, err := io.ReadFull(rw, methods); err != nil {
		return err
	}

	if slices.Contains(methods, methodNoAuth) {
		_, err := rw.Write([]byte{version5, methodNoAuth})
		return err
	}

	_, _ = rw.Write([]byte{version5, methodNoAcceptable})
	return errors.New("client offered no acceptable authentication method")
}

// readRequest parses a request. On error, reply is the code to answer with, or ReplySucceeded
// when the stream is too broken to answer at all.
func readRequest(r io.Reader) (host string, port int, reply byte, err error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return "", 0, ReplySucceeded, err
	}

	if hdr[0] != version5 {
		return "", 0, ReplySucceeded, fmt.Errorf("unsupported SOCKS version %d", hdr[0])
	}

	switch hdr[3] {
	case atypIPv4:
		var ip [net.IPv4len]byte
		if _, err := io.ReadFull(r, ip[:]); err != nil {
			return "", 0, ReplySucceeded, err
		}
		host = net.IP(ip[:]).String()
	case atypIPv6:
		var ip [net.IPv6len]byte
		if _, err := io.ReadFull(r, ip[:]); err != nil {
			return "", 0, ReplySucceeded, err
		}
		host = net.IP(ip[:]).String()
	case atypDomain:
		var n [1]byte
		if _, err := io.ReadFull(r, n[:]); err != nil {
			return "", 0, ReplySucceeded, err
		}
		if n[0] == 0 {
			return "", 0, ReplyGeneralFailure, errors.New("empty domain name")
		}
		name := make([]byte, n[0])
		if _, err := io.ReadFull(r, name); err != nil {
			return "", 0, ReplySucceeded, err
		}
		host = string(name)
	default:
		return "", 0, ReplyAddressNotSupported, fmt.Errorf("unsupported address type %d", hdr[3])
	}

	var p [2]byte
	if _, err := io.ReadFull(r, p[:]); err != nil {
		return "", 0, ReplySucceeded, err
	}
	port = int(binary.BigEndian.Uint16(p[:]))

	// checked after the full request is read, so the reply isn't interleaved with unread input
	if hdr[1] != cmdConnect {
		return "", 0, ReplyCommandNotSupported, fmt.Errorf("unsupported command %d", hdr[1])
	}

	return host, port, ReplySucceeded, nil
}

// writeReply answers a request. bound is the server-side address of the outbound connection,
// reported as 0.0.0.0:0 when nil.
func writeReply(w io.Writer, code byte, bound net.Addr) error {
	ip := net.IPv4zero.To4()
	port := 0

	if tcp, ok := bound.(*net.TCPAddr); ok && tcp != nil {
		port = tcp.Port
		if v4 := tcp.IP.To4(); v4 != nil {
			ip = v4
		} else if tcp.IP != nil {
			ip = tcp.IP.To16()
		}
	}

	atyp := byte(atypIPv4)
	if len(ip) == net.IPv6len {
		atyp = atypIPv6
	}

	msg := make([]byte, 0, 6+len(ip))
	msg = append(msg, version5, code, 0x00, atyp)
	msg = append(msg, ip...)
	msg = binary.BigEndian.AppendUint16(msg, uint16(port))

	_, err := w.Write(msg)
	return err
}

// replyFor maps a dial error to the closest SOCKS5 reply code.
func replyFor(err error) byte {
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return ReplyConnectionRefused
	case errors.Is(err, syscall.ENETUNREACH):
		return ReplyNetworkUnreachable
	case errors.Is(err, syscall.EHOSTUNREACH):
		return ReplyHostUnreachable
	case errors.Is(err, context.DeadlineExceeded):
		return ReplyTTLExpired
	}

	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return ReplyHostUnreachable
	}

	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return ReplyTTLExpired
	}

	return ReplyGeneralFailure
}

// relay copies in both directions until both have finished, half-closing each side as its
// source reaches EOF so request/response protocols see a clean end of stream.
func relay(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	pipe := func(dst, src net.Conn) {
		defer wg.Done()

		_, _ = io.Copy(dst, src)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		} else {
			_ = dst.Close()
		}
	}

	go pipe(a, b)
	go pipe(b, a)
	wg.Wait()
}

func (s *Server) track(l net.Listener) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false
	}
	if s.listeners == nil {
		s.listeners = make(map[net.Listener]struct{})
	}
	s.listeners[l] = struct{}{}
	return true
}

func (s *Server) untrack(l net.Listener) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.listeners, l)
}

func (s *Server) trackConn(c net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false
	}
	if s.conns == nil {
		s.conns = make(map[net.Conn]struct{})
	}
	s.conns[c] = struct{}{}
	return true
}

func (s *Server) untrackConn(c net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.conns, c)
}

func (s *Server) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.closed
}

func (s *Server) infof(format string, args ...any) {
	if s.Log != nil {
		s.Log.Infof(format, args...)
	}
}

func (s *Server) debugf(format string, args ...any) {
	if s.Log != nil {
		s.Log.Debugf(format, args...)
	}
}
