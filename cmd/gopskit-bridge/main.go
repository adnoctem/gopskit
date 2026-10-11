// gopskit-bridge is the in-cluster half of the gopskit bridge: a SOCKS5 server that lets the
// gopskit CLIs reach cluster-internal services such as Keycloak and Vault by their real service
// names, through a single Kubernetes port-forward.
//
// It listens on the pod's loopback interface only. Nothing in the cluster network can reach it;
// the only way in is a port-forward, which Kubernetes RBAC already controls.
//
// Usage:
//
//	gopskit-bridge [--port 1080]        serve
//	gopskit-bridge probe [--port 1080]  exit 0 if the local server completes a SOCKS5 greeting
//
// The probe subcommand exists for the kubelet: a tcpSocket probe targets the pod IP and cannot
// reach a loopback-only listener, and the image has no shell to run anything else.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/adnoctem/gopskit/pkg/bridge/socks5"
	"github.com/adnoctem/gopskit/pkg/log"
	_ "github.com/adnoctem/gopskit/pkg/stamp"
)

// DefaultPort is the SOCKS5 port the bridge listens on inside its pod.
const DefaultPort = 1080

func main() {
	if len(os.Args) > 1 && os.Args[1] == "probe" {
		os.Exit(probe(os.Args[2:]))
	}

	os.Exit(serve(os.Args[1:]))
}

func serve(args []string) int {
	fs := flag.NewFlagSet("gopskit-bridge", flag.ExitOnError)
	port := fs.Int("port", DefaultPort, "loopback port to listen on")
	_ = fs.Parse(args)

	lgr := log.New()
	defer func() { _ = lgr.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listeners, err := listenLoopback(*port)
	if err != nil {
		lgr.Errorf("could not listen: %v", err)
		return 1
	}

	srv := &socks5.Server{Log: lgr}

	var wg sync.WaitGroup
	errs := make(chan error, len(listeners))
	for _, l := range listeners {
		lgr.Infof("gopskit-bridge listening on %s", l.Addr())

		wg.Go(func() {
			if err := srv.Serve(ctx, l); !errors.Is(err, socks5.ErrServerClosed) {
				errs <- err
				stop()
			}
		})
	}
	wg.Wait()
	close(errs)

	if err := <-errs; err != nil {
		lgr.Errorf("server stopped: %v", err)
		return 1
	}

	lgr.Info("gopskit-bridge stopped")
	return 0
}

// listenLoopback listens on 127.0.0.1, which is required, and on ::1 where the pod has IPv6.
// Port-forwards connect to "localhost" inside the pod's network namespace, which may resolve to
// either address.
func listenLoopback(port int) ([]net.Listener, error) {
	v4, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}

	listeners := []net.Listener{v4}
	if v6, err := net.Listen("tcp6", net.JoinHostPort("::1", strconv.Itoa(port))); err == nil {
		listeners = append(listeners, v6)
	}

	return listeners, nil
}

func probe(args []string) int {
	fs := flag.NewFlagSet("gopskit-bridge probe", flag.ExitOnError)
	port := fs.Int("port", DefaultPort, "loopback port the server listens on")
	timeout := fs.Duration("timeout", 2*time.Second, "how long to wait for the server")
	_ = fs.Parse(args)

	if err := greet(net.JoinHostPort("127.0.0.1", strconv.Itoa(*port)), *timeout); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "probe failed: %v\n", err)
		return 1
	}

	return 0
}

// greet completes a SOCKS5 method negotiation, which proves the server is accepting and
// answering, without asking it to connect anywhere.
func greet(addr string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return err
	}

	var resp [2]byte
	if _, err := io.ReadFull(conn, resp[:]); err != nil {
		return err
	}

	if resp != [2]byte{0x05, 0x00} {
		return fmt.Errorf("unexpected greeting response %#v", resp)
	}

	return nil
}
