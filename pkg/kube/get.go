package kube

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Client) Namespaces(opts metav1.ListOptions) ([]corev1.Namespace, error) {
	l, err := c.Client.CoreV1().Namespaces().List(context.Background(), opts)
	return items(l, err, func(l *corev1.NamespaceList) []corev1.Namespace { return l.Items })
}

func (c *Client) Pods(namespace string, opts metav1.ListOptions) ([]corev1.Pod, error) {
	l, err := c.Client.CoreV1().Pods(namespace).List(context.Background(), opts)
	return items(l, err, func(l *corev1.PodList) []corev1.Pod { return l.Items })
}

func (c *Client) Service(namespace, name string, opts metav1.GetOptions) (*corev1.Service, error) {
	return get(c.Client.CoreV1().Services(namespace).Get(context.Background(), name, opts))
}

func (c *Client) Services(namespace string, opts metav1.ListOptions) ([]corev1.Service, error) {
	l, err := c.Client.CoreV1().Services(namespace).List(context.Background(), opts)
	return items(l, err, func(l *corev1.ServiceList) []corev1.Service { return l.Items })
}

func (c *Client) ConfigMap(namespace, name string, opts metav1.GetOptions) (*corev1.ConfigMap, error) {
	return get(c.Client.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, opts))
}

func (c *Client) ConfigMaps(namespace string, opts metav1.ListOptions) ([]corev1.ConfigMap, error) {
	l, err := c.Client.CoreV1().ConfigMaps(namespace).List(context.Background(), opts)
	return items(l, err, func(l *corev1.ConfigMapList) []corev1.ConfigMap { return l.Items })
}

func (c *Client) Secret(namespace string, name string, opts metav1.GetOptions) (*corev1.Secret, error) {
	return get(c.Client.CoreV1().Secrets(namespace).Get(context.Background(), name, opts))
}

func (c *Client) Secrets(namespace string, opts metav1.ListOptions) ([]corev1.Secret, error) {
	l, err := c.Client.CoreV1().Secrets(namespace).List(context.Background(), opts)
	return items(l, err, func(l *corev1.SecretList) []corev1.Secret { return l.Items })
}

func (c *Client) Ingresses(namespace string, opts metav1.ListOptions) ([]networkingv1.Ingress, error) {
	l, err := c.Client.NetworkingV1().Ingresses(namespace).List(context.Background(), opts)
	return items(l, err, func(l *networkingv1.IngressList) []networkingv1.Ingress { return l.Items })
}

func (c *Client) StorageClass(name string, opts metav1.GetOptions) (*storagev1.StorageClass, error) {
	return get(c.Client.StorageV1().StorageClasses().Get(context.Background(), name, opts))
}
