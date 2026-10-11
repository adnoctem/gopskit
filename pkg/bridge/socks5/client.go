package socks5

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

// Greet completes a method negotiation with the SOCKS5 server at addr, proving it is accepting
// and answering, without asking it to connect anywhere.
func Greet(ctx context.Context, addr string) error {
	conn, err := dialServer(ctx, addr)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	return clientNegotiate(conn)
}

// Dial asks the SOCKS5 server at proxyAddr to connect to target ("host:port") and returns the
// tunnelled connection. host may be a domain name, which the server resolves. A refusal is
// returned as a *ReplyError carrying the server's reply code.
func Dial(ctx context.Context, proxyAddr, target string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return nil, err
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid port in %q: %w", target, err)
	}

	conn, err := dialServer(ctx, proxyAddr)
	if err != nil {
		return nil, err
	}

	if err := clientConnect(conn, host, uint16(port)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	// the handshake deadline must not outlive the handshake
	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

// ReplyError is a request the server answered with a reply code other than ReplySucceeded.
type ReplyError struct {
	Code byte
}

func (e *ReplyError) Error() string {
	return fmt.Sprintf("socks5: server refused the request: %s", replyText(e.Code))
}

func dialServer(ctx context.Context, addr string) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(DefaultHandshakeTimeout)
	}
	_ = conn.SetDeadline(deadline)

	return conn, nil
}

func clientNegotiate(rw io.ReadWriter) error {
	if _, err := rw.Write([]byte{version5, 1, methodNoAuth}); err != nil {
		return err
	}

	var resp [2]byte
	if _, err := io.ReadFull(rw, resp[:]); err != nil {
		return err
	}

	if resp[0] != version5 || resp[1] != methodNoAuth {
		return fmt.Errorf("socks5: unexpected method selection %#x %#x", resp[0], resp[1])
	}

	return nil
}

func clientConnect(rw io.ReadWriter, host string, port uint16) error {
	if err := clientNegotiate(rw); err != nil {
		return err
	}

	msg := []byte{version5, cmdConnect, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			msg = append(msg, atypIPv4)
			msg = append(msg, v4...)
		} else {
			msg = append(msg, atypIPv6)
			msg = append(msg, ip.To16()...)
		}
	} else {
		if len(host) == 0 || len(host) > 255 {
			return fmt.Errorf("socks5: invalid host name %q", host)
		}
		msg = append(msg, atypDomain, byte(len(host)))
		msg = append(msg, host...)
	}
	msg = binary.BigEndian.AppendUint16(msg, port)

	if _, err := rw.Write(msg); err != nil {
		return err
	}

	var hdr [4]byte
	if _, err := io.ReadFull(rw, hdr[:]); err != nil {
		return err
	}

	// consume the bound address, whatever its type, so the stream starts at the tunnelled data
	var skip int
	switch hdr[3] {
	case atypIPv4:
		skip = net.IPv4len
	case atypIPv6:
		skip = net.IPv6len
	case atypDomain:
		var n [1]byte
		if _, err := io.ReadFull(rw, n[:]); err != nil {
			return err
		}
		skip = int(n[0])
	default:
		return fmt.Errorf("socks5: unexpected address type %#x in reply", hdr[3])
	}
	if _, err := io.ReadFull(rw, make([]byte, skip+2)); err != nil {
		return err
	}

	if hdr[1] != ReplySucceeded {
		return &ReplyError{Code: hdr[1]}
	}

	return nil
}

func replyText(code byte) string {
	switch code {
	case ReplyGeneralFailure:
		return "general failure"
	case ReplyNotAllowed:
		return "connection not allowed by ruleset"
	case ReplyNetworkUnreachable:
		return "network unreachable"
	case ReplyHostUnreachable:
		return "host unreachable"
	case ReplyConnectionRefused:
		return "connection refused"
	case ReplyTTLExpired:
		return "timed out"
	case ReplyCommandNotSupported:
		return "command not supported"
	case ReplyAddressNotSupported:
		return "address type not supported"
	default:
		return fmt.Sprintf("reply code %#x", code)
	}
}
