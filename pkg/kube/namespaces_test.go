package kube

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/util/retry"
)

var namespaceResource = schema.GroupResource{Resource: "namespaces"}

func TestReconcileNamespaceIntent(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "create"
		if existing {
			name = "existing"
		}
		t.Run(name, func(t *testing.T) {
			cs := fake.NewSimpleClientset()
			if existing {
				require.NoError(t, cs.Tracker().Add(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "example", ResourceVersion: "123"}}))
			}
			cs.PrependReactor("patch", "namespaces", func(action clienttesting.Action) (bool, runtime.Object, error) {
				patch := action.(clienttesting.PatchActionImpl)
				assert.Equal(t, types.ApplyPatchType, patch.GetPatchType())
				assert.Equal(t, NamespaceFieldManager, patch.GetPatchOptions().FieldManager)
				require.NotNil(t, patch.GetPatchOptions().Force)
				assert.False(t, *patch.GetPatchOptions().Force)
				var intent map[string]any
				require.NoError(t, json.Unmarshal(patch.GetPatch(), &intent))
				meta := map[string]any{"name": "example", "labels": map[string]any{"env": "dev"}}
				if existing {
					meta["resourceVersion"] = "123"
				}
				assert.Equal(t, map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": meta}, intent)
				return true, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "example"}}, nil
			})
			c := &Client{Client: cs}
			require.NoError(t, c.ReconcileNamespace(t.Context(), metav1.ObjectMeta{Name: "example", Labels: map[string]string{"env": "dev"}}))
			require.Len(t, cs.Actions(), 2)
			assert.Equal(t, "get", cs.Actions()[0].GetVerb())
			assert.Equal(t, "patch", cs.Actions()[1].GetVerb())
		})
	}
}

func TestReconcileNamespaceErrorsAndRaces(t *testing.T) {
	conflict := apierrors.NewConflict(namespaceResource, "example", errors.New("stale version"))
	owned := apierrors.NewApplyConflict([]metav1.StatusCause{{Type: metav1.CauseTypeFieldManagerConflict}}, "owned by someone else")
	for _, tt := range []struct {
		name, verb string
		err        error
		recover    bool
		attempts   int
	}{
		{name: "forbidden get", verb: "get", err: apierrors.NewForbidden(namespaceResource, "example", errors.New("forbidden")), attempts: 1},
		{name: "transport error", verb: "get", err: errors.New("connection refused"), attempts: 1},
		{name: "admission rejected", verb: "patch", err: apierrors.NewForbidden(namespaceResource, "example", errors.New("admission rejected")), attempts: 1},
		{name: "ownership conflict", verb: "patch", err: owned, attempts: 1},
		{name: "creation race", verb: "patch", err: apierrors.NewAlreadyExists(namespaceResource, "example"), recover: true, attempts: 2},
		{name: "version race", verb: "patch", err: conflict, recover: true, attempts: 2},
		{name: "retry exhausted", verb: "patch", err: conflict, attempts: retry.DefaultBackoff.Steps},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cs := fake.NewSimpleClientset()
			attempts := 0
			cs.PrependReactor(tt.verb, "namespaces", func(_ clienttesting.Action) (bool, runtime.Object, error) {
				attempts++
				if tt.recover && attempts > 1 {
					return true, &corev1.Namespace{}, nil
				}
				return true, nil, tt.err
			})
			err := (&Client{Client: cs}).ReconcileNamespace(t.Context(), metav1.ObjectMeta{Name: "example"})
			if tt.recover {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.err)
				assert.Contains(t, err.Error(), `namespace "example"`)
			}
			assert.Equal(t, tt.attempts, attempts)
			for _, action := range cs.Actions() {
				assert.Contains(t, []string{"get", "patch"}, action.GetVerb())
			}
		})
	}
}

func TestReconcileNamespaceTermination(t *testing.T) {
	for _, afterApply := range []bool{false, true} {
		for _, deletionTimestamp := range []bool{false, true} {
			cs := fake.NewSimpleClientset()
			live := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "example"}}
			if deletionTimestamp {
				now := metav1.Now()
				live.DeletionTimestamp = &now
			} else {
				live.Status.Phase = corev1.NamespaceTerminating
			}
			verb := "get"
			if afterApply {
				verb = "patch"
			}
			cs.PrependReactor(verb, "namespaces", func(_ clienttesting.Action) (bool, runtime.Object, error) { return true, live, nil })
			err := (&Client{Client: cs}).ReconcileNamespace(t.Context(), metav1.ObjectMeta{Name: "example"})
			require.ErrorContains(t, err, "terminating")
			if !afterApply {
				assert.Len(t, cs.Actions(), 1, "must not modify a terminating namespace")
			}
		}
	}
}

func TestReconcileNamespaceCancellation(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx, cancel := context.WithCancel(t.Context())
	cs.PrependReactor("patch", "namespaces", func(_ clienttesting.Action) (bool, runtime.Object, error) {
		cancel()
		return true, nil, apierrors.NewConflict(namespaceResource, "example", errors.New("stale version"))
	})
	c := &Client{Client: cs}
	require.ErrorIs(t, c.ReconcileNamespace(ctx, metav1.ObjectMeta{Name: "example"}), context.Canceled)
	cs.ClearActions()
	require.ErrorIs(t, c.ReconcileNamespace(ctx, metav1.ObjectMeta{Name: "example"}), context.Canceled)
	assert.Empty(t, cs.Actions())
}
