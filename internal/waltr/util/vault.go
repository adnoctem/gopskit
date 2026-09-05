package util

import (
	"context"
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
	return filepath.Join(a.Paths.Cache, env.String(), "vault-credentials.json")
}

// WriteCredentials persists the given Vault credentials to the CredentialPath for env, and makes
// them the active token for subsequent Vault API calls.
func WriteCredentials(a *app.State, env core.Environment, credentials *apivault.Credentials) error {
	a.Vault.SetAuthPath(CredentialPath(a, env))
	if err := a.Vault.SetToken(credentials); err != nil {
		return err
	}

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

// AuthMethods retrieves the list of enabled authentication methods from the current Vault instance
func AuthMethods(a *app.State) ([]string, error) {
	m, err := a.Vault.AuthListEnabledMethods(context.Background())
	if err != nil {
		return nil, fmt.Errorf("could not list enabled Vault authentication methods: %v", err)
	}

	methods := make([]string, 0, len(m))
	for k := range m {
		methods = append(methods, k)
	}

	return methods, nil
}

// SecretsEngines retrieves the list of enabled secrets engines from the current Vault instance
func SecretsEngines(a *app.State) ([]string, error) {
	s, err := a.Vault.MountsListSecretsEngines(context.Background())
	if err != nil {
		return nil, fmt.Errorf("could not list secrets engines: %v", err)
	}

	engines := make([]string, 0, len(s))
	for k := range s {
		engines = append(engines, k)
	}

	return engines, nil
}

// Policies retrieves a list of the currently enabled policies
func Policies(a *app.State) ([]string, error) {
	p, err := a.Vault.PoliciesListAclPolicies(context.Background())
	if err != nil {
		return nil, fmt.Errorf("could not list policies: %v", err)
	}

	return p, nil
}

// PasswordPolicies retrieves a list of the currently enabled password policies
func PasswordPolicies(a *app.State) ([]string, error) {
	p, err := a.Vault.PoliciesListPasswordPolicies(context.Background())
	if err != nil {
		// mitigate empty policies
		if apivault.IsNotFound(err) {
			return []string{}, nil
		}

		return nil, fmt.Errorf("could not list password policies: %v", err)
	}

	return p, nil
}

// KubernetesAuthRoles retrieves a list of the currently enabled Kubernetes Auth roles within Vault
func KubernetesAuthRoles(a *app.State) ([]string, error) {
	k, err := a.Vault.KubernetesListAuthRoles(context.Background())
	if err != nil {
		// mitigate empty policies
		if apivault.IsNotFound(err) {
			return []string{}, nil
		}

		return nil, fmt.Errorf("could not list Kubernetes Auth roles: %v", err)
	}

	return k, nil
}

// GeneratePasswordFromPolicy generates a new password using the named Vault password policy
func GeneratePasswordFromPolicy(a *app.State, policy string) (string, error) {
	pass, err := a.Vault.PoliciesGeneratePasswordFromPasswordPolicy(context.Background(), policy)
	if err != nil {
		return "", fmt.Errorf("could not generate password: %v", err)
	}

	return pass, nil
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

// HasKvV2Secret reports whether a KV-v2 secret exists at path, mounted at mountPath
func HasKvV2Secret(a *app.State, path, mountPath string) bool {
	_, err := a.Vault.KvV2Read(context.Background(), mountPath, path)
	return err == nil
}

// WriteKvV2Secret writes (or overwrites) a KV-v2 secret at path, mounted at mountPath
func WriteKvV2Secret(a *app.State, path, mountPath string, data map[string]interface{}) error {
	return a.Vault.KvV2Write(context.Background(), mountPath, path, data)
}

// Policies
var (
	Releases = []string{
		"keycloak",
		"awx",
		"crowdsec",
		"gitlab",
		"gitlab-runner",
		"harbor",
		"headlamp",
		"homepage",
		"jenkins",
		"kubescape",
		"loki",
		"matomo",
	}

	ConfigReleasePolicyTemplate = `path "kv/data/%s/*" {
   capabilities = ["read"]
}`

	ConfigAclPolicies = map[string]string{
		"admin": `# Read system health check
path "sys/health"
{
  capabilities = ["read", "sudo"]
}

# Create and manage ACL policies broadly across Vault

# List existing policies
path "sys/policies/acl"
{
  capabilities = ["list"]
}

# Create and manage ACL policies
path "sys/policies/acl/*"
{
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# Enable and manage authentication methods broadly across Vault

# Manage auth methods broadly across Vault
path "auth/*"
{
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# Create, update, and delete auth methods
path "sys/auth/*"
{
  capabilities = ["create", "update", "delete", "sudo"]
}

# List auth methods
path "sys/auth"
{
  capabilities = ["read"]
}

# Enable and manage the key/value secrets engine at \"secret\" path

# List, create, update, and delete key/value secrets
path "kv/*"
{
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# List, create, update, and delete transit secrets
path "transit/*"
{
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# List, create, update, and delete cubbyhole secrets
path "cubbyhole/*"
{
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# List, create, update, and delete Vault identities
path "identity/*"
{
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# Manage secrets engines
path "sys/mounts/*"
{
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# List existing secrets engines.
path "sys/mounts"
{
  capabilities = ["read"]
}`,
	}

	ConfigPasswordPolicies = map[string]string{
		"alphanumeric-password": `length = 64
rule "charset" {
  charset = "abcdefghijklmnopqrstuvwxyz"
  min-chars = 16
}
rule "charset" {
  charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
  min-chars = 16
}
rule "charset" {
  charset = "0123456789"
  min-chars = 16
}`,
		"alphanumeric-special-password": `length = 64
rule "charset" {
  charset = "abcdefghijklmnopqrstuvwxyz"
  min-chars = 12
}
rule "charset" {
  charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
  min-chars = 12
}
rule "charset" {
  charset = "0123456789"
  min-chars = 12
}
rule "charset" {
  charset = "!@#$%^&*"
  min-chars = 12
}`,
		"s3-access-key": `length = 32
rule "charset" {
  charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
  min-chars = 16
}
rule "charset" {
  charset = "0123456789"
  min-chars = 12
}`,
		"s3-secret-key": `length = 64
rule "charset" {
  charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
  min-chars = 32
}
rule "charset" {
  charset = "0123456789"
  min-chars = 24
}`,
	}

	TRANSIT_ENCRYPTION_POLICY = `path "transit/encrypt/vso-client-cache" {
   capabilities = ["create", "update"]
}
path "transit/decrypt/vso-client-cache" {
   capabilities = ["create", "update"]
}`
)
