package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/adnoctem/gopskit/internal/carto/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// newState is carto's state with no kubeconfig anywhere, so any command that touched the
// cluster would fail.
func newState(t *testing.T) *app.State {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("KUBECONFIG", "")
	t.Chdir(t.TempDir())

	s, err := app.New()
	require.NoError(t, err)

	return s
}

func write(t *testing.T, path, content string) string {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestHelpNeedsNoCluster(t *testing.T) {
	root := NewRootCommand(newState(t))
	root.SetArgs([]string{"apply", "--help"})
	root.SetOut(&discard{})

	assert.NoError(t, root.Execute())
}

// render works offline, which makes it the place to check that settings reach the commands from
// carto.yaml - including --backbone, which used to be --config.
func TestRenderWithSettingsFromConfigFile(t *testing.T) {
	asrt := assert.New(t)
	s := newState(t)
	dir := t.TempDir()

	backbone := write(t, filepath.Join(dir, "infra", "backbone.yaml"), `
globals:
  - path: globals.yaml
    applies: ["*"]
charts:
  vault:
    version: 0.30.0
    repository: https://helm.releases.hashicorp.com
    namespace: vault
`)
	write(t, filepath.Join(dir, "infra", "globals.yaml"), "replicas: 1\nlog: info\n")
	write(t, filepath.Join(dir, "values", "vault", "values.yaml"), "replicas: 3\n")
	write(t, filepath.Join(dir, "values", "vault", "values.prod.yaml"), "log: debug\n")

	out := filepath.Join(dir, "rendered.yaml")
	cfg := write(t, filepath.Join(dir, "carto.yaml"), `
backbone: `+backbone+`
charts-dir: `+filepath.Join(dir, "values")+`
environment: prod
render:
  output: `+out+`
`)

	root := NewRootCommand(s)
	root.SetArgs([]string{"render", "vault", "--config", cfg})
	require.NoError(t, root.Execute())

	rendered, err := os.ReadFile(out)
	require.NoError(t, err)
	asrt.Contains(string(rendered), "replicas: 3", "the chart's values must override the globals")
	asrt.Contains(string(rendered), "log: debug", "the environment overlay must apply")
}

func TestPickPod(t *testing.T) {
	asrt := assert.New(t)

	pod := func(name string, phase corev1.PodPhase, ready, deleting bool) corev1.Pod {
		p := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.PodStatus{Phase: phase}}
		if ready {
			p.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
		}
		if deleting {
			p.DeletionTimestamp = &metav1.Time{Time: time.Now()}
		}
		return p
	}

	got, ok := pickPod([]corev1.Pod{
		pod("a-terminating", corev1.PodRunning, true, true),
		pod("b-pending", corev1.PodPending, false, false),
		pod("d-ready", corev1.PodRunning, true, false),
		pod("c-running", corev1.PodRunning, false, false),
	})
	require.True(t, ok)
	asrt.Equal("d-ready", got.Name, "a ready pod wins over a merely running one")

	got, ok = pickPod([]corev1.Pod{pod("z", corev1.PodRunning, false, false), pod("y", corev1.PodRunning, false, false)})
	require.True(t, ok)
	asrt.Equal("y", got.Name, "ties go to the first name, so the choice is stable")

	_, ok = pickPod([]corev1.Pod{pod("gone", corev1.PodRunning, true, true), pod("waiting", corev1.PodPending, false, false)})
	asrt.False(ok, "terminating and pending pods are never picked")
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
