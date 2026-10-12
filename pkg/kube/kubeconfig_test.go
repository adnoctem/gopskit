package kube

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeKubeConfig writes a kubeconfig with two contexts, "one" (current) and "two", whose
// servers are https://<name>.<label>.example so tests can tell which file and context won.
func writeKubeConfig(t *testing.T, path, label string) string {
	t.Helper()

	content := fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: one
clusters:
- name: one
  cluster: {server: "https://one.%[1]s.example"}
- name: two
  cluster: {server: "https://two.%[1]s.example"}
users:
- name: user
  user: {token: t}
contexts:
- name: one
  context: {cluster: one, user: user}
- name: two
  context: {cluster: two, user: user}
`, label)

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// isolate points the default kubeconfig locations at empty temporary directories.
func isolate(t *testing.T) (home, configDir string) {
	t.Helper()

	home, configDir = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("KUBECONFIG", "")

	return home, configDir
}

func TestKubeConfigFromEnvironment(t *testing.T) {
	isolate(t)
	env := writeKubeConfig(t, filepath.Join(t.TempDir(), "env-config"), "env")
	t.Setenv("KUBECONFIG", env)

	kc, err := NewClient()
	require.NoError(t, err)
	assert.Equal(t, "https://one.env.example", kc.Config.Host)
}

func TestKubeConfigHomeFile(t *testing.T) {
	home, _ := isolate(t)
	writeKubeConfig(t, filepath.Join(home, ".kube", "config"), "home")

	kc, err := NewClient()
	require.NoError(t, err)
	assert.Equal(t, "https://one.home.example", kc.Config.Host)
}

func TestExplicitPathBeatsEnvironment(t *testing.T) {
	isolate(t)
	t.Setenv("KUBECONFIG", writeKubeConfig(t, filepath.Join(t.TempDir(), "env-config"), "env"))
	explicit := writeKubeConfig(t, filepath.Join(t.TempDir(), "explicit"), "explicit")

	kc, err := NewClient(WithConfigPath(explicit))
	require.NoError(t, err)
	assert.Equal(t, "https://one.explicit.example", kc.Config.Host)
}

func TestGopskitFallback(t *testing.T) {
	_, configDir := isolate(t)
	writeKubeConfig(t, filepath.Join(configDir, "gopskit", "kubeconfig"), "gopskit")

	kc, err := NewClient()
	require.NoError(t, err)
	assert.Equal(t, "https://one.gopskit.example", kc.Config.Host)
}

func TestContextSelection(t *testing.T) {
	isolate(t)
	t.Setenv("KUBECONFIG", writeKubeConfig(t, filepath.Join(t.TempDir(), "env-config"), "env"))

	kc, err := NewClient(WithContext("two"))
	require.NoError(t, err)
	assert.Equal(t, "https://two.env.example", kc.Config.Host)

	// the Helm SDK builds its own REST config from Flags; it must land on the same cluster
	helmConfig, err := kc.Flags.ToRESTConfig()
	require.NoError(t, err)
	assert.Equal(t, "https://two.env.example", helmConfig.Host)
}

func TestUnknownContextFails(t *testing.T) {
	isolate(t)
	t.Setenv("KUBECONFIG", writeKubeConfig(t, filepath.Join(t.TempDir(), "env-config"), "env"))

	_, err := NewClient(WithContext("three"))
	assert.Error(t, err)
}

func TestNoKubeConfigFails(t *testing.T) {
	isolate(t)
	t.Setenv("KUBERNETES_SERVICE_HOST", "")

	_, err := NewClient()
	assert.ErrorContains(t, err, "kubeconfig")
}
