package util

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/adnoctem/gopskit/internal/waltr/app"
	apivault "github.com/adnoctem/gopskit/pkg/api/vault"
	"github.com/adnoctem/gopskit/pkg/core"
	"github.com/adnoctem/gopskit/pkg/helpers"
	"github.com/hashicorp/hcl/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VaultConfig struct {
	DisableMLock bool        `hcl:"disable_mlock"`
	UI           bool        `hcl:"ui"`
	Seal         *SealConfig `hcl:"seal,block"`
	Remain       hcl.Body    `hcl:",remain"`
}

type SealConfig struct {
	Type   string   `hcl:"type,label"`
	Remain hcl.Body `hcl:",remain"`
}

// CredentialPath builds the filesystem path to write the credentials to after we unseal the Vault,
// since it most likely is required for later commands. This function make the path deterministic
// per execution env.Environment.
func CredentialPath(a *app.State, env core.Environment) string {
	if a.CredentialFile != "" {
		return a.CredentialFile
	}

	return filepath.Join(a.Paths.Cache, env.String(), "vault-credentials.json")
}

// WriteCredentials persists the given Vault credentials to the CredentialPath for env, and makes
// them the active token for subsequent Vault API calls.
func WriteCredentials(a *app.State, env core.Environment, credentials *apivault.Credentials) error {
	// SetToken replaces the client's credentials with these, so the path is set afterwards, on them
	if err := a.Vault.SetToken(credentials); err != nil {
		return err
	}
	a.Vault.SetAuthPath(CredentialPath(a, env))

	return a.Vault.Token().Save()
}

// ReadCredentials reads previously-persisted Vault credentials from the CredentialPath for env
func ReadCredentials(a *app.State, env core.Environment) (*apivault.Credentials, error) {
	a.Vault.SetAuthPath(CredentialPath(a, env))

	creds := a.Vault.Token()
	if err := creds.Load(); err != nil {
		return nil, err
	}

	asserted, ok := creds.(*apivault.Credentials)
	if !ok {
		return nil, fmt.Errorf("loaded Vault credentials have an unexpected type")
	}

	return asserted, nil
}

// Pods returns a list of Kubernetes' Pods matching the default (or custom) Vault label
func Pods(a *app.State, namespace, label string) ([]corev1.Pod, error) {
	if label == "" {
		a.Log.Debugf("no label provided for Pods search, using default label: %s", app.DefaultLabel)
		label = app.DefaultLabel
	}

	pods, err := a.Kube.Pods(namespace, metav1.ListOptions{
		LabelSelector: label,
	})
	a.Log.Debugf("found %d pods for label: %s", len(pods), label)

	if err != nil {
		return nil, err
	}

	return pods, nil
}

// LeaderPod determines the Vault leader (active) Pod from a set of candidate pods, preferring the
// "vault-active=true" label and falling back to the StatefulSet's ordinal-0 replica.
func LeaderPod(a *app.State, pods []corev1.Pod, namespace, label string) (*corev1.Pod, error) {
	var activePods []corev1.Pod
	var err error

	if len(pods) > 1 {
		label = fmt.Sprintf("%s,%s", label, "vault-active=true")
		activePods, err = a.Kube.Pods(namespace, metav1.ListOptions{
			LabelSelector: label,
		})

		if err != nil {
			return nil, err
		}
	}

	// matched more than one
	if len(activePods) > 1 {
		return nil, fmt.Errorf("could not determine Vault leader pod. "+
			"Invalid configuration: Kubernetes label %s matched more than one pod", label)
	}

	// no pod is labeled with "vault-active=true"
	if len(activePods) == 0 {
		for i := range pods {
			if strings.HasSuffix(pods[i].Name, "-0") {
				return &pods[i], nil
			}
		}

		return nil, fmt.Errorf("could not determine Vault leader pod. Unfamiliar naming scheme. " +
			"None of your Vault Pod names end in the StatefulSet ordinal '-0'")
	}

	return &activePods[0], nil
}

// EnsureNamespace ensures we only find and use Vault Pods within a single namespace
func EnsureNamespace(pods []corev1.Pod) (string, error) {
	if len(pods) == 0 {
		return "", fmt.Errorf("no Vault pods found")
	}

	var ns []string
	for _, pod := range pods {
		ns = append(ns, pod.Namespace)
	}

	rns := helpers.RemoveDuplicates(ns)
	if len(rns) > 1 {
		return "", fmt.Errorf("discovered Vault pods in multiple namespaces: %v! Please set the namespace option", rns)
	}

	return rns[0], nil
}

// WaitUntilRunning blocks until the given Pod's current status (re-fetched from the cluster on
// every iteration) reports as Running.
func WaitUntilRunning(a *app.State, pod corev1.Pod) error {
	for {
		current, err := a.Kube.Pods(pod.Namespace, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("metadata.name=%s", pod.Name),
		})
		if err != nil {
			return err
		}

		if len(current) == 0 {
			return fmt.Errorf("pod %s no longer exists in namespace %s", pod.Name, pod.Namespace)
		}

		if current[0].Status.Phase == corev1.PodRunning {
			a.Log.Infof("Vault Pod: %s is running", pod.Name)
			return nil
		}

		a.Log.Infof("Vault Pod: %s is not running yet - waiting for Pod to start", pod.Name)
		time.Sleep(2500 * time.Millisecond)
	}
}
