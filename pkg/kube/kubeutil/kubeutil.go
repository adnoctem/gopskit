// kubeutil implements utilities for working with Kubernetes API objects.
package kubeutil

import (
	"fmt"
	"strings"
	"time"

	"github.com/fmjstudios/gopskit/pkg/helpers"
	"github.com/fmjstudios/gopskit/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WaitUntilRunning waits until the specified Pod is running and returns the time it took to wait
// for it to become running.
func WaitUntilRunning(pod *corev1.Pod) time.Duration {
	now := time.Now()

	for {
		if pod.Status.Phase == corev1.PodRunning {
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	end := time.Now()
	return end.Sub(now)
}

// EnsureNamespace ensures that all Pods in a list belong to the same namespace. If it is found that
// all Pods belong to a singular namespace, that namespace's name is returned.
func EnsureNamespace(pods []corev1.Pod) (string, error) {
	ns := make([]string, 0)
	for _, pod := range pods {
		ns = append(ns, pod.Namespace)
	}

	rns := helpers.RemoveDuplicates(ns)
	if len(rns) > 1 {
		return "", fmt.Errorf("found Pods in multiple namespaces: [%s]", strings.Join(rns, ", "))
	}

	return ns[0], nil
}

// LeaderPod searches the cluster (namespace) for Pods matching one or both of the provided labels.
// If both labels are supplied both are used, otherwise only the podLabel will be used. If either
// combination returns none or more than a single Pod, a non-nil error is returned.
func LeaderPod(client *kube.Client, namespace, podLabel, selectorLabel string) (*corev1.Pod, error) {
	var pods []corev1.Pod
	var searchLabel string

	if selectorLabel != "" {
		searchLabel = strings.Join([]string{podLabel, selectorLabel}, ",")
	} else {
		searchLabel = podLabel
	}

	pods, err := client.Pods(namespace, v1.ListOptions{
		LabelSelector: searchLabel,
	})
	if err != nil {
		return nil, err
	}

	if len(pods) == 0 {
		return nil, fmt.Errorf("search with combined labelSelector: %s resulted in zero Pods returned", searchLabel)
	}

	if len(pods) > 1 {
		return nil, fmt.Errorf("could not determine leader pod with combined labelSelector: %s", searchLabel)
	}

	return &pods[0], nil
}

// func ParseBasicAuthSecret(client *kube.Client, name, namespace string) (username, password string, err error) {
// 	secret, err := client.Secret(namespace, name, metav1.GetOptions{})
// 	if err != nil {
// 		return "", "", fmt.Errorf("could not find Secret '%s' in namespace: %s. error: %v", name, namespace, err)
// 	}

// 	return string(secret.Data["username"]), string(secret.Data["password"]), nil
// }

func ParseSecretKeys(secret *corev1.Secret, keys ...string) (map[string]string, error) {
	res := make(map[string]string)
	for _, key := range keys {
		value, ok := secret.Data[key]
		if !ok {
			return nil, fmt.Errorf("secret %s does not contain the key: %s", secret.Name, key)
		}
		res[key] = string(value)
	}

	return res, nil
}
