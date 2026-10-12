package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestContainerName(t *testing.T) {
	pod := corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "sidecar"}, {Name: "vault"}}}}

	got, err := containerName(pod, "")
	require.NoError(t, err)
	assert.Equal(t, "sidecar", got, "without a name, the first container is used")

	got, err = containerName(pod, "vault")
	require.NoError(t, err)
	assert.Equal(t, "vault", got)

	_, err = containerName(pod, "missing")
	assert.ErrorContains(t, err, "no container")

	_, err = containerName(corev1.Pod{}, "")
	assert.Error(t, err)
}
