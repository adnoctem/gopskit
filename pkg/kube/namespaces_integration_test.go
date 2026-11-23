package kube_test

import (
	"testing"

	"github.com/adnoctem/gopskit/pkg/kube"
	"github.com/adnoctem/gopskit/pkg/kube/k3stest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applyv1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// The API server, rather than a fake object tracker, must prove ownership and
// omission semantics. This cluster is disposable and never uses the user's context.
func TestNamespaceSSA(t *testing.T) {
	_, kc := k3stest.Start(t)
	assertNamespaceOwnership(t, kc)
	// A finalizer keeps this disposable test namespace terminating for the check.
	require.NoError(t, kc.Client.CoreV1().Namespaces().Delete(t.Context(), "existing", metav1.DeleteOptions{}))
	err := kc.ReconcileNamespace(t.Context(), metav1.ObjectMeta{Name: "existing"})
	require.ErrorContains(t, err, "terminating")
}

// NewClientset simulates managed fields, unlike NewSimpleClientset. Keep this
// contract runnable without Docker as well as exercising it against the real API.
func TestNamespaceFieldManagement(t *testing.T) {
	assertNamespaceOwnership(t, &kube.Client{Client: fake.NewClientset()})
}

func assertNamespaceOwnership(t *testing.T, kc *kube.Client) {
	t.Helper()
	ctx := t.Context()
	namespaces := kc.Client.CoreV1().Namespaces()
	other := applyv1.Namespace("existing").
		WithLabels(map[string]string{"other": "keep", "shared": "same"}).
		WithAnnotations(map[string]string{"other": "keep"}).
		WithFinalizers("example.org/keep")
	before, err := namespaces.Apply(ctx, other, metav1.ApplyOptions{FieldManager: "other-actor"})
	require.NoError(t, err)

	meta := metav1.ObjectMeta{
		Name: "existing", Labels: map[string]string{"carto": "first", "shared": "same"},
		Annotations: map[string]string{"carto": "first"},
	}
	require.NoError(t, kc.ReconcileNamespace(ctx, meta))
	require.NoError(t, kc.ReconcileNamespace(ctx, meta), "repeated apply must converge")
	meta.Labels["carto"] = "second"
	meta.Annotations["carto"] = "second"
	require.NoError(t, kc.ReconcileNamespace(ctx, meta))
	live, err := namespaces.Get(ctx, meta.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "second", live.Labels["carto"])
	assert.Equal(t, "second", live.Annotations["carto"])
	assert.Equal(t, before.UID, live.UID)
	assert.Equal(t, before.CreationTimestamp, live.CreationTimestamp)
	assert.Equal(t, before.Spec, live.Spec)
	assert.Equal(t, before.Status, live.Status)
	assert.Equal(t, before.Finalizers, live.Finalizers)
	assert.Equal(t, "keep", live.Labels["other"])
	assert.Equal(t, "keep", live.Annotations["other"])
	if value, ok := before.Labels["kubernetes.io/metadata.name"]; ok {
		assert.Equal(t, value, live.Labels["kubernetes.io/metadata.name"])
	}

	meta.Labels["other"] = "takeover"
	err = kc.ReconcileNamespace(ctx, meta)
	require.True(t, apierrors.IsConflict(err), "expected an ownership conflict, got %v", err)
	live, err = namespaces.Get(ctx, meta.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "keep", live.Labels["other"])

	// Omit every Carto key: only keys owned solely by Carto may disappear.
	require.NoError(t, kc.ReconcileNamespace(ctx, metav1.ObjectMeta{Name: meta.Name}))
	live, err = namespaces.Get(ctx, meta.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotContains(t, live.Labels, "carto")
	assert.NotContains(t, live.Annotations, "carto")
	assert.Equal(t, "same", live.Labels["shared"])
	assert.Equal(t, "keep", live.Labels["other"])
	assert.Equal(t, "keep", live.Annotations["other"])
	assert.Equal(t, before.Spec, live.Spec)
	assert.Equal(t, before.Finalizers, live.Finalizers)

	require.NoError(t, kc.ReconcileNamespace(ctx, metav1.ObjectMeta{Name: "new-namespace"}))
	created, err := namespaces.Get(ctx, "new-namespace", metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, kc.ReconcileNamespace(ctx, metav1.ObjectMeta{Name: "new-namespace"}))
	repeated, err := namespaces.Get(ctx, "new-namespace", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, created.UID, repeated.UID)

}
