package kube

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/fmjstudios/gopskit/pkg/proc"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
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
	var stdOut, stdErr bytes.Buffer

	dialer, err := createDialer(method, url, restConfig)
	if err != nil {
		return err
	}

	pf, err := portforward.NewOnAddresses(
		dialer,
		[]string{"localhost"},
		ports,
		stopChan,
		readyChan,
		&stdOut,
		&stdErr,
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
