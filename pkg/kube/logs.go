package kube

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
)

// Logs opens a stream of the given Pod's logs. The caller is responsible for closing the
// returned io.ReadCloser once done reading.
func (c *Client) Logs(namespace, name string, opts corev1.PodLogOptions) (io.ReadCloser, error) {
	return c.Client.CoreV1().Pods(namespace).GetLogs(name, &opts).Stream(context.Background())
}
