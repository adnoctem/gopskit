package kube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"github.com/adnoctem/gopskit/pkg/proc"
	corev1 "k8s.io/api/core/v1"

	// client-go's portforward.NewOnAddresses/NewFallbackDialer/NewSPDYOverWebsocketDialer still
	// require this exact type as of client-go v0.36.2, not the newer k8s.io/streaming package -
	// keeping this import is the only way to satisfy their signatures.
	"k8s.io/apimachinery/pkg/util/httpstream" //nolint:staticcheck
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type PortForwarder interface {
	ForwardPorts(method string, restConfig *rest.Config, url *url.URL, ports []string, stopChan,
		readyChan chan struct{}) error
}

type DefaultPortForwarder struct{}

func (*DefaultPortForwarder) ForwardPorts(method string, restConfig *rest.Config, url *url.URL, ports []string, stopChan,
	readyChan chan struct{}) error {
	var err error

	dialer, err := createDialer(method, url, restConfig)
	if err != nil {
		return err
	}

	// client-go writes a line per connection from that connection's own goroutine, so the
	// writers must be safe for concurrent use; nothing reads them, hence io.Discard
	pf, err := portforward.NewOnAddresses(
		dialer,
		[]string{"localhost"},
		ports,
		stopChan,
		readyChan,
		io.Discard,
		io.Discard,
	)
	if err != nil {
		return err
	}

	return pf.ForwardPorts()
}

// PortForward port-forwards a remote port of a Kubernetes container to the local machine
func (c *Client) PortForward(ctx context.Context, pod corev1.Pod, readyChan chan struct{}) error {
	if pod.Status.Phase != corev1.PodRunning {
		return fmt.Errorf("unable to forward port of a non-running Pod. current status: %v", pod.Status.Phase)
	}

	// create control channels
	stopChan := make(chan struct{})
	nctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// allow CTRL+C
	go proc.AwaitCancel(func() int {
		cancel()
		return 0
	})

	go func() {
		<-nctx.Done()
		close(stopChan)
	}()

	req := c.Client.CoreV1().
		RESTClient().
		Post().
		Resource("pods").
		Namespace(pod.Namespace).
		Name(pod.Name).
		SubResource("portforward")

	return c.PortForwarder.ForwardPorts(
		"POST",
		c.Config,
		req.URL(),
		BuildDefaultPortMap(pod.Spec.Containers[0].Ports[0].ContainerPort),
		stopChan,
		readyChan,
	)
}

func createDialer(method string, url *url.URL, restConfig *rest.Config) (httpstream.Dialer, error) {
	var dialer httpstream.Dialer
	var err error

	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return nil, err
	}

	spdyDialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, method, url)
	dialer, err = portforward.NewSPDYOverWebsocketDialer(url, restConfig)
	if err != nil {
		return nil, err
	}

	dialer = portforward.NewFallbackDialer(dialer, spdyDialer, func(err error) bool {
		return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
	})

	return dialer, nil
}

// only require remoteport as input
func BuildDefaultPortMap(remotePort int32) []string {
	return []string{fmt.Sprintf("%s:%d", DefaultLocalPort, remotePort)}
}

// Forward is an open port-forward to a single Pod port, bound to a random free port on the local
// loopback interface. Unlike PortForward it never blocks the caller beyond establishing the
// tunnel, and installs no signal handling of its own.
type Forward struct {
	// LocalPort is the port on 127.0.0.1 that reaches the Pod
	LocalPort uint16

	stop chan struct{}
	once sync.Once
	done chan error
}

// Close tears the port-forward down and waits for it to finish. It is safe to call more than once.
func (f *Forward) Close() error {
	f.once.Do(func() { close(f.stop) })

	err, ok := <-f.done
	if !ok {
		return nil
	}

	return err
}

// Done is closed when the port-forward ends, e.g. because the Pod went away.
func (f *Forward) Done() <-chan error {
	return f.done
}

// Forward opens a port-forward from a random free local port to remotePort on pod. It returns
// once the tunnel is ready, or with an error if it could not be established before ctx ended.
func (c *Client) Forward(ctx context.Context, pod corev1.Pod, remotePort int) (*Forward, error) {
	if pod.Status.Phase != corev1.PodRunning {
		return nil, fmt.Errorf("unable to forward port of a non-running Pod %s/%s. current status: %v",
			pod.Namespace, pod.Name, pod.Status.Phase)
	}

	req := c.Client.CoreV1().
		RESTClient().
		Post().
		Resource("pods").
		Namespace(pod.Namespace).
		Name(pod.Name).
		SubResource("portforward")

	dialer, err := createDialer("POST", req.URL(), c.Config)
	if err != nil {
		return nil, err
	}

	f := &Forward{stop: make(chan struct{}), done: make(chan error, 1)}
	ready := make(chan struct{})

	// see DefaultPortForwarder.ForwardPorts for why the output is discarded
	pf, err := portforward.NewOnAddresses(
		dialer,
		[]string{"127.0.0.1"},
		[]string{fmt.Sprintf("0:%d", remotePort)},
		f.stop,
		ready,
		io.Discard,
		io.Discard,
	)
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(f.done)

		if err := pf.ForwardPorts(); err != nil {
			f.done <- err
		}
	}()

	select {
	case <-ready:
	case err := <-f.done:
		if err == nil {
			err = fmt.Errorf("port-forward to %s/%s ended before it became ready", pod.Namespace, pod.Name)
		}
		return nil, err
	case <-ctx.Done():
		_ = f.Close()
		return nil, ctx.Err()
	}

	ports, err := pf.GetPorts()
	if err != nil || len(ports) != 1 {
		_ = f.Close()
		return nil, fmt.Errorf("could not determine the local port of the port-forward: %v", err)
	}
	f.LocalPort = ports[0].Local

	return f, nil
}
