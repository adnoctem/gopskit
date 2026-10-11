package socks5

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startServer runs srv on a loopback listener and returns its address. The server is shut down
// when the test ends.
func startServer(t *testing.T, srv *Server) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background(), l) }()

	t.Cleanup(func() {
		_ = srv.Shutdown()
		assert.ErrorIs(t, <-done, ErrServerClosed)
	})

	return l.Addr().String()
}

// recordingDialer dials 127.0.0.1:<port> for every request while recording the address the
// server asked for, so tests can assert on unresolved domain names without real DNS.
type recordingDialer struct {
	mu     sync.Mutex
	asked  []string
	target string
}

func (d *recordingDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	d.mu.Lock()
	d.asked = append(d.asked, address)
	d.mu.Unlock()

	var dialer net.Dialer
	return dialer.DialContext(ctx, network, d.target)
}

func (d *recordingDialer) addresses() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return append([]string(nil), d.asked...)
}

func httpClientVia(proxyAddr string) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(&url.URL{Scheme: "socks5", Host: proxyAddr}),
		},
	}
}

// TestHTTPThroughProxy is the end-to-end case the CLIs rely on: net/http's built-in SOCKS5
// support, a cluster-style hostname the client cannot resolve itself, and the server resolving
// it on the client's behalf.
func TestHTTPThroughProxy(t *testing.T) {
	asrt := assert.New(t)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "host=%s", r.Host)
	}))
	t.Cleanup(backend.Close)

	dialer := &recordingDialer{target: backend.Listener.Addr().String()}
	addr := startServer(t, &Server{Dialer: dialer})

	resp, err := httpClientVia(addr).Get("http://keycloak.keycloak.svc:8080/realms/master")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	asrt.Equal(http.StatusOK, resp.StatusCode)
	asrt.Equal("host=keycloak.keycloak.svc:8080", string(body))
	asrt.Equal([]string{"keycloak.keycloak.svc:8080"}, dialer.addresses(),
		"the domain name must reach the server unresolved, so the cluster's DNS answers it")
}

func TestAllowRefusesRequest(t *testing.T) {
	asrt := assert.New(t)

	dialer := &recordingDialer{target: "127.0.0.1:1"}
	addr := startServer(t, &Server{
		Dialer: dialer,
		Allow:  func(host string, port int) bool { return host != "forbidden.svc" },
	})

	reply := rawConnect(t, addr, "forbidden.svc", 443)
	asrt.Equal(ReplyNotAllowed, reply)
	asrt.Empty(dialer.addresses(), "a refused request must never be dialled")
}

func TestDialFailureMapsToReply(t *testing.T) {
	asrt := assert.New(t)

	// grab a free port and close it again, so connecting to it is refused
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closedPort := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())

	addr := startServer(t, &Server{})
	asrt.Equal(ReplyConnectionRefused, rawConnect(t, addr, "127.0.0.1", closedPort))
}

func TestUnsupportedCommand(t *testing.T) {
	asrt := assert.New(t)

	addr := startServer(t, &Server{})
	conn := dial(t, addr)
	handshake(t, conn)

	// BIND (0x02) to 127.0.0.1:80
	_, err := conn.Write([]byte{version5, 0x02, 0x00, atypIPv4, 127, 0, 0, 1, 0, 80})
	require.NoError(t, err)

	asrt.Equal(ReplyCommandNotSupported, readReply(t, conn))
}

func TestUnsupportedAddressType(t *testing.T) {
	asrt := assert.New(t)

	addr := startServer(t, &Server{})
	conn := dial(t, addr)
	handshake(t, conn)

	_, err := conn.Write([]byte{version5, cmdConnect, 0x00, 0x09})
	require.NoError(t, err)

	asrt.Equal(ReplyAddressNotSupported, readReply(t, conn))
}

func TestRejectsClientWithoutNoAuthMethod(t *testing.T) {
	asrt := assert.New(t)

	addr := startServer(t, &Server{})
	conn := dial(t, addr)

	// offer username/password (0x02) only
	_, err := conn.Write([]byte{version5, 1, 0x02})
	require.NoError(t, err)

	var resp [2]byte
	_, err = io.ReadFull(conn, resp[:])
	require.NoError(t, err)
	asrt.Equal([2]byte{version5, methodNoAcceptable}, resp)
}

func TestHandshakeTimeout(t *testing.T) {
	asrt := assert.New(t)

	addr := startServer(t, &Server{HandshakeTimeout: 50 * time.Millisecond})
	conn := dial(t, addr)

	// say nothing; the server must hang up on its own
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err := conn.Read(make([]byte, 1))
	asrt.ErrorIs(err, io.EOF)
}

func TestShutdownClosesOpenTunnels(t *testing.T) {
	asrt := assert.New(t)

	// a backend that accepts and then never says anything, so the tunnel stays open
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = backend.Close() })
	go func() {
		for {
			c, err := backend.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = c.Close() })
		}
	}()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := &Server{}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background(), l) }()

	port := backend.Addr().(*net.TCPAddr).Port
	conn := dial(t, l.Addr().String())
	handshake(t, conn)
	writeConnect(t, conn, "127.0.0.1", port)
	require.Equal(t, ReplySucceeded, readReply(t, conn))

	require.NoError(t, srv.Shutdown())
	asrt.ErrorIs(<-done, ErrServerClosed)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Read(make([]byte, 1))
	asrt.Error(err, "an open tunnel must be closed by Shutdown")
	asrt.False(errors.Is(err, context.DeadlineExceeded))
}

func TestServeStopsWhenContextIsCancelled(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- (&Server{}).Serve(ctx, l) }()

	cancel()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, ErrServerClosed)
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after its context was cancelled")
	}
}

func TestWriteReplyAddressTypes(t *testing.T) {
	asrt := assert.New(t)

	var v4 bytesWriter
	require.NoError(t, writeReply(&v4, ReplySucceeded, &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 4242}))
	asrt.Equal([]byte{version5, ReplySucceeded, 0, atypIPv4, 10, 0, 0, 7, 0x10, 0x92}, v4.b)

	var v6 bytesWriter
	require.NoError(t, writeReply(&v6, ReplySucceeded, &net.TCPAddr{IP: net.ParseIP("fd00::1"), Port: 1}))
	asrt.Equal(byte(atypIPv6), v6.b[3])
	asrt.Len(v6.b, 4+net.IPv6len+2)

	var none bytesWriter
	require.NoError(t, writeReply(&none, ReplyGeneralFailure, nil))
	asrt.Equal([]byte{version5, ReplyGeneralFailure, 0, atypIPv4, 0, 0, 0, 0, 0, 0}, none.b)
}

type bytesWriter struct{ b []byte }

func (w *bytesWriter) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}

// ---- raw protocol helpers ----

func dial(t *testing.T, addr string) net.Conn {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	return conn
}

func handshake(t *testing.T, conn net.Conn) {
	t.Helper()

	_, err := conn.Write([]byte{version5, 1, methodNoAuth})
	require.NoError(t, err)

	var resp [2]byte
	_, err = io.ReadFull(conn, resp[:])
	require.NoError(t, err)
	require.Equal(t, [2]byte{version5, methodNoAuth}, resp)
}

func writeConnect(t *testing.T, conn net.Conn, host string, port int) {
	t.Helper()

	msg := []byte{version5, cmdConnect, 0x00}
	if ip := net.ParseIP(host).To4(); ip != nil {
		msg = append(msg, atypIPv4)
		msg = append(msg, ip...)
	} else {
		msg = append(msg, atypDomain, byte(len(host)))
		msg = append(msg, host...)
	}
	msg = binary.BigEndian.AppendUint16(msg, uint16(port))

	_, err := conn.Write(msg)
	require.NoError(t, err)
}

func readReply(t *testing.T, conn net.Conn) byte {
	t.Helper()

	var hdr [4]byte
	_, err := io.ReadFull(conn, hdr[:])
	require.NoError(t, err)

	n := net.IPv4len
	if hdr[3] == atypIPv6 {
		n = net.IPv6len
	}
	_, err = io.ReadFull(conn, make([]byte, n+2))
	require.NoError(t, err)

	return hdr[1]
}

func rawConnect(t *testing.T, addr, host string, port int) byte {
	t.Helper()

	conn := dial(t, addr)
	handshake(t, conn)
	writeConnect(t, conn, host, port)

	return readReply(t, conn)
}

func TestClientDialThroughServer(t *testing.T) {
	asrt := assert.New(t)

	backend, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = backend.Close() })
	go func() {
		c, err := backend.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		_, _ = io.Copy(c, c)
	}()

	dialer := &recordingDialer{target: backend.Addr().String()}
	addr := startServer(t, &Server{Dialer: dialer})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, Greet(ctx, addr))

	conn, err := Dial(ctx, addr, "vault.vault.svc:8200")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	_, err = conn.Write([]byte("ping"))
	require.NoError(t, err)
	got := make([]byte, 4)
	_, err = io.ReadFull(conn, got)
	require.NoError(t, err)

	asrt.Equal("ping", string(got), "data must flow through the tunnel in both directions")
	asrt.Equal([]string{"vault.vault.svc:8200"}, dialer.addresses())
}

func TestClientDialReportsRefusal(t *testing.T) {
	asrt := assert.New(t)

	addr := startServer(t, &Server{Allow: func(string, int) bool { return false }})

	_, err := Dial(context.Background(), addr, "vault.vault.svc:8200")

	var replyErr *ReplyError
	require.ErrorAs(t, err, &replyErr)
	asrt.Equal(ReplyNotAllowed, replyErr.Code)
	asrt.Contains(err.Error(), "not allowed")
}
