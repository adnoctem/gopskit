package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	applyv1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/util/retry"
)

// NamespaceFieldManager identifies Carto's metadata intent to server-side apply.
const NamespaceFieldManager = "carto-namespaces"

// ReconcileNamespace creates or reconciles explicitly supplied namespace metadata.
// Only name, labels and annotations are submitted. Other actors' fields are preserved;
// ownership conflicts fail rather than forcing a takeover. Omitted metadata keys formerly
// owned solely by Carto are removed by server-side apply. Namespaces are never deleted.
func (c *Client) ReconcileNamespace(ctx context.Context, meta metav1.ObjectMeta) error {
	if meta.Name == "" {
		return fmt.Errorf("namespace name is required")
	}
	namespaces := c.Client.CoreV1().Namespaces()
	var lastErr error
	err := wait.ExponentialBackoffWithContext(ctx, retry.DefaultBackoff, func(ctx context.Context) (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		intent := applyv1.Namespace(meta.Name).WithLabels(meta.Labels).WithAnnotations(meta.Annotations)
		live, err := namespaces.Get(ctx, meta.Name, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
			// Apply also creates. A simultaneous creator is subject to the same
			// non-forced ownership rules as any existing namespace.
		case err != nil:
			return false, fmt.Errorf("get: %w", err)
		default:
			if err := namespaceActive(live); err != nil {
				return false, err
			}
			intent.WithResourceVersion(live.ResourceVersion)
		}
		result, err := namespaces.Apply(ctx, intent, metav1.ApplyOptions{FieldManager: NamespaceFieldManager})
		if err != nil {
			lastErr = fmt.Errorf("apply: %w", err)
			if apierrors.IsAlreadyExists(err) || (apierrors.IsConflict(err) &&
				!apierrors.HasStatusCause(err, metav1.CauseTypeFieldManagerConflict)) {
				return false, nil
			}
			return false, lastErr
		}
		return true, namespaceActive(result)
	})
	if wait.Interrupted(err) && ctx.Err() == nil && lastErr != nil {
		err = lastErr
	}
	if err != nil {
		return fmt.Errorf("could not reconcile namespace %q: %w", meta.Name, err)
	}
	return nil
}

func namespaceActive(ns *corev1.Namespace) error {
	if ns.DeletionTimestamp != nil || ns.Status.Phase == corev1.NamespaceTerminating {
		return fmt.Errorf("namespace is terminating; resolve its deletion before applying")
	}
	return nil
}
