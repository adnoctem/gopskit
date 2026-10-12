package cmd

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/adnoctem/gopskit/internal/plattr/app"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// reflectorAnnotations are the annotations that have emberstack's Reflector mirror a Secret into
// namespaces, or none when reflect is false.
func reflectorAnnotations(reflect bool, namespaces []string) map[string]string {
	if !reflect {
		return map[string]string{}
	}

	ns := strings.Join(namespaces, ",")
	return map[string]string{
		"reflector.v1.k8s.emberstack.com/reflection-allowed":            "true",
		"reflector.v1.k8s.emberstack.com/reflection-allowed-namespaces": ns,
		"reflector.v1.k8s.emberstack.com/reflection-auto-enabled":       "true",
		"reflector.v1.k8s.emberstack.com/reflection-auto-namespaces":    ns,
	}
}

// secretExists reports whether the Secret exists. Only "not found" counts as missing: any other
// error - no permission, an unreachable cluster - is returned, so it can't be mistaken for absence.
func secretExists(ctx context.Context, a *app.State, namespace, name string) (bool, error) {
	kc, err := a.KubeClient()
	if err != nil {
		return false, err
	}

	_, err = kc.Client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	switch {
	case err == nil:
		return true, nil
	case apierrors.IsNotFound(err):
		return false, nil
	default:
		return false, fmt.Errorf("could not look up Secret %s/%s: %w", namespace, name, err)
	}
}

// applySecret creates secret. An existing Secret is left as it is unless overwrite is set, which
// replaces its data, labels and annotations - for rotating credentials. what names the Secret in
// messages.
func applySecret(ctx context.Context, a *app.State, secret *corev1.Secret, overwrite bool, what string) error {
	kc, err := a.KubeClient()
	if err != nil {
		return err
	}
	secrets := kc.Client.CoreV1().Secrets(secret.Namespace)

	have, err := secrets.Get(ctx, secret.Name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		if _, err := secrets.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("could not create the %s Secret %s/%s: %w", what, secret.Namespace, secret.Name, err)
		}
		a.Log.Infof("created the %s Secret %s/%s", what, secret.Namespace, secret.Name)
		return nil
	case err != nil:
		return fmt.Errorf("could not look up Secret %s/%s: %w", secret.Namespace, secret.Name, err)
	case !overwrite:
		a.Log.Infof("the %s Secret %s/%s already exists; --overwrite replaces it", what, secret.Namespace, secret.Name)
		return nil
	}

	// the type of an existing Secret can't change, so it's kept
	have.Data = secret.Data
	have.StringData = secret.StringData
	have.Labels = maps.Clone(secret.Labels)
	have.Annotations = maps.Clone(secret.Annotations)

	if _, err := secrets.Update(ctx, have, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("could not update the %s Secret %s/%s: %w", what, secret.Namespace, secret.Name, err)
	}
	a.Log.Infof("replaced the %s Secret %s/%s", what, secret.Namespace, secret.Name)
	return nil
}

// ensureStorageClass creates sc unless it exists. Kubernetes doesn't allow changing a
// StorageClass's provisioner or parameters, so an existing one is left as it is.
func ensureStorageClass(ctx context.Context, a *app.State, sc *storagev1.StorageClass, what string) error {
	kc, err := a.KubeClient()
	if err != nil {
		return err
	}
	classes := kc.Client.StorageV1().StorageClasses()

	_, err = classes.Get(ctx, sc.Name, metav1.GetOptions{})
	switch {
	case err == nil:
		a.Log.Infof("the %s StorageClass %s already exists", what, sc.Name)
		return nil
	case !apierrors.IsNotFound(err):
		return fmt.Errorf("could not look up StorageClass %s: %w", sc.Name, err)
	}

	if _, err := classes.Create(ctx, sc, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("could not create the %s StorageClass %s: %w", what, sc.Name, err)
	}
	a.Log.Infof("created the %s StorageClass %s", what, sc.Name)
	return nil
}
