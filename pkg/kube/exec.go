package kube

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/moby/term"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/streaming/pkg/httpstream"
)

// RemoteExecutor defines the interface accepted by the Exec command - provided for test stubbing
type RemoteExecutor interface {
	Execute(url *url.URL, restConfig *rest.Config, stdout, stderr io.Writer) error
}

// DefaultRemoteExecutor is the standard implementation of remote command execution
type DefaultRemoteExecutor struct{}

// Execute implements the RemoteExecutor interface for the DefaultRemoteExecutor
func (*DefaultRemoteExecutor) Execute(url *url.URL, restConfig *rest.Config, stdout, stderr io.Writer) error {
	exec, err := createExecutor(url, restConfig)
	if err != nil {
		return err
	}

	return exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})
}

// Exec executes a command within the container of a specific Pod in the current namespace
// configured for the client. If another namespace is required the ExecInNamespace helper
// provides an escape hatch
func (c *Client) Exec(command string, pod corev1.Pod) (string, string, error) {
	var err error
	var stdOut, stdErr bytes.Buffer

	args := strings.Fields(command)
	req := c.Client.CoreV1().
		RESTClient().
		Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: pod.Spec.Containers[0].Name,
			Command:   args,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	if err = c.Executor.Execute(req.URL(), c.Config, &stdOut, &stdErr); err != nil {
		return "", "", fmt.Errorf("error in Kubernetes exec Stream: %v", err.Error())
	}

	return stdOut.String(), stdErr.String(), nil
}

// fixedSizeQueue reports a single, unchanging terminal size and then stops - it does not track
// live resize events (SIGWINCH). Adding that is future work.
type fixedSizeQueue struct {
	size *remotecommand.TerminalSize
	sent bool
}

// Next implements remotecommand.TerminalSizeQueue for fixedSizeQueue
func (q *fixedSizeQueue) Next() *remotecommand.TerminalSize {
	if q.sent {
		return nil
	}

	q.sent = true
	return q.size
}

// ExecTTY executes an interactive command within the given container of pod, attaching the
// current process's stdin/stdout/stderr with a TTY and reporting the initial terminal size to the
// remote side.
func (c *Client) ExecTTY(pod corev1.Pod, container string, command []string) error {
	stdIn, stdOut, stdErr := term.StdStreams()

	req := c.Client.CoreV1().
		RESTClient().
		Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	exec, err := createExecutor(req.URL(), c.Config)
	if err != nil {
		return err
	}

	var sizeQueue remotecommand.TerminalSizeQueue
	if fd, isTerminal := term.GetFdInfo(stdIn); isTerminal {
		if ws, err := term.GetWinsize(fd); err == nil {
			sizeQueue = &fixedSizeQueue{
				size: &remotecommand.TerminalSize{Width: ws.Width, Height: ws.Height},
			}
		}
	}

	return exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdin:             stdIn,
		Stdout:            stdOut,
		Stderr:            stdErr,
		Tty:               true,
		TerminalSizeQueue: sizeQueue,
	})
}

// createExecutor creates a Kubernetes remote Executor for use with the Exec client methods. The function is scoped
// to WebSocketExecutors and as such isn't compatible with the Kubernetes KUBECTL_REMOTE_COMMAND_WEBSOCKETS feature
// gate.
//
// NOTE: This function is largely analogous to the implementation within the `kubectl exec` command
// ref: https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/kubectl/pkg/cmd/exec/exec.go#L138
func createExecutor(url *url.URL, config *rest.Config) (remotecommand.Executor, error) {
	var exec remotecommand.Executor
	var err error

	// Fallback executor is default, unless feature flag is explicitly disabled.
	exec, err = remotecommand.NewWebSocketExecutor(config, "GET", url.String())
	if err != nil {
		return nil, err
	}

	exec, err = remotecommand.NewFallbackExecutor(exec, exec, func(err error) bool {
		return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
	})

	if err != nil {
		return nil, err
	}

	return exec, nil
}
