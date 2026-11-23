package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/adnoctem/gopskit/internal/carto/app"
	apihelm "github.com/adnoctem/gopskit/pkg/helm"
	"github.com/adnoctem/gopskit/pkg/kube"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

const declaredNamespaces = "namespaces:\n  - name: one\n    labels: {env: dev}\n  - name: two\n"

type namespaceHelm struct {
	events *[]string
	fail   string
	hook   string
	t      *testing.T
}

func (h *namespaceHelm) step(name string) error {
	*h.events = append(*h.events, name)
	if h.fail == name {
		return fmt.Errorf("%s failed", name)
	}
	return nil
}

func (h *namespaceHelm) LoadChart(_, _, _ string) (*chart.Chart, error) {
	return &chart.Chart{}, h.step("load")
}

func (h *namespaceHelm) Installed(_ string) (bool, error) {
	return true, h.step("installed")
}

func (h *namespaceHelm) Apply(_ string, _ *chart.Chart, _ map[string]interface{}, _ ...apihelm.ApplyOption) (*release.Release, error) {
	if h.hook != "" {
		_, err := os.Stat(h.hook)
		require.NoError(h.t, err, "pre-apply hook must run before Helm")
	}
	return &release.Release{Version: 2}, h.step("helm")
}

func (h *namespaceHelm) RenderManifest(_ string, _ *chart.Chart, _ map[string]interface{}) (string, error) {
	return "same manifest", h.step("render")
}

func (h *namespaceHelm) CurrentManifest(_ string) (string, error) {
	return "same manifest", h.step("current")
}

func namespaceFixture(t *testing.T, declarations string) (*app.State, *fake.Clientset, *namespaceHelm, string, string) {
	t.Helper()
	a := newState(t)
	cs := fake.NewSimpleClientset()
	a.Kube = &kube.Client{Client: cs, Flags: genericclioptions.NewConfigFlags(true)}
	missingConfig := filepath.Join(t.TempDir(), "missing-kubeconfig")
	a.Kube.Flags.KubeConfig = &missingConfig
	events := []string{}
	h := &namespaceHelm{events: &events, t: t}
	cs.PrependReactor("patch", "namespaces", func(action clienttesting.Action) (bool, runtime.Object, error) {
		name := action.(clienttesting.PatchAction).GetName()
		if err := h.step("namespace:" + name); err != nil {
			return true, nil, err
		}
		return true, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}, nil
	})
	cs.PrependReactor("list", "pods", func(_ clienttesting.Action) (bool, runtime.Object, error) {
		_ = h.step("pods")
		return true, &corev1.PodList{}, nil
	})
	dir := t.TempDir()
	backbone := write(t, filepath.Join(dir, "backbone.yaml"), declarations+"charts:\n  example:\n    namespace: unmanaged\n")
	return a, cs, h, backbone, dir
}

func applyRoot(a *app.State, h *namespaceHelm) *cobra.Command {
	root := NewRootCommand(a)
	old, _, _ := root.Find([]string{"apply"})
	root.RemoveCommand(old)
	root.AddCommand(newApplyCommand(a, func(_ string) (applyHelm, error) { return h, nil }))
	return root
}

func TestApplyNamespaceOrderAndFailure(t *testing.T) {
	for _, failure := range []string{"", "load", "installed", "namespace:two", "hook", "helm"} {
		t.Run("failure="+failure, func(t *testing.T) {
			a, cs, h, backbone, dir := namespaceFixture(t, declaredNamespaces)
			h.fail = failure
			h.hook = filepath.Join(dir, "hook-ran")
			exit := 0
			if failure == "hook" {
				exit = 1
			}
			script := write(t, filepath.Join(dir, "hook.sh"), fmt.Sprintf("#!/bin/sh\ntouch %q\nexit %d\n", h.hook, exit))
			require.NoError(t, os.Chmod(script, 0700))
			write(t, backbone, declaredNamespaces+fmt.Sprintf("charts:\n  example:\n    namespace: unmanaged\n    hooks:\n      pre-apply:\n        - local: %q\n", script))
			root := applyRoot(a, h)
			root.SetArgs([]string{"apply", "example", "-b", backbone, "--charts-dir", dir})
			err := root.Execute()
			if failure == "" {
				require.NoError(t, err)
				assert.Equal(t, []string{"load", "installed", "namespace:one", "namespace:two", "pods", "helm", "pods"}, *h.events)
			} else {
				require.Error(t, err)
				if failure != "helm" {
					assert.NotContains(t, *h.events, "helm")
				}
				if failure == "load" || failure == "installed" {
					assert.Empty(t, cs.Actions())
				}
				if failure == "namespace:two" {
					_, err := os.Stat(h.hook)
					assert.True(t, os.IsNotExist(err), "namespace failure must stop hooks")
				}
			}
			for _, action := range cs.Actions() {
				if action.GetResource().Resource == "namespaces" {
					assert.Contains(t, []string{"get", "patch"}, action.GetVerb(), "no failure cleanup")
				}
			}
		})
	}
}

func TestApplyDoesNotInferNamespaces(t *testing.T) {
	for _, declaration := range []string{"", "namespaces: []\n", "namespaces: null\n", declaredNamespaces} {
		t.Run(declaration, func(t *testing.T) {
			a, cs, h, backbone, dir := namespaceFixture(t, declaration)
			for range 2 {
				root := applyRoot(a, h)
				root.SetArgs([]string{"apply", "example", "-b", backbone, "--charts-dir", dir})
				require.NoError(t, root.Execute())
			}
			patches := 0
			for _, action := range cs.Actions() {
				if action.GetResource().Resource != "namespaces" {
					continue
				}
				switch action := action.(type) {
				case clienttesting.PatchAction:
					patches++
					assert.Contains(t, []string{"one", "two"}, action.GetName())
				case clienttesting.GetAction:
					assert.NotEqual(t, "unmanaged", action.GetName())
				default:
					t.Fatalf("unexpected namespace action: %v", action)
				}
			}
			if declaration == declaredNamespaces {
				assert.Equal(t, 4, patches, "each apply reconciles all explicit declarations")
			} else {
				assert.Zero(t, patches)
				for _, action := range cs.Actions() {
					assert.NotEqual(t, "namespaces", action.GetResource().Resource)
				}
			}
		})
	}
}

func TestApplyValidatesBeforeNamespaceWrites(t *testing.T) {
	for _, failure := range []string{"namespaces", "chart", "values"} {
		t.Run(failure, func(t *testing.T) {
			a, cs, h, backbone, dir := namespaceFixture(t, declaredNamespaces)
			name := "example"
			switch failure {
			case "namespaces":
				write(t, backbone, "namespaces: [{name: INVALID}]\ncharts: {example: {}}")
			case "chart":
				name = "unknown"
			case "values":
				write(t, filepath.Join(dir, "example", "values.yaml"), "broken: [")
			}
			root := applyRoot(a, h)
			root.SetArgs([]string{"apply", name, "-b", backbone, "--charts-dir", dir})
			require.Error(t, root.Execute())
			assert.Empty(t, cs.Actions())
			assert.Empty(t, *h.events)
		})
	}
}

func TestNamespaceDiffPreview(t *testing.T) {
	for _, declaration := range []string{"", declaredNamespaces} {
		a, cs, h, backbone, dir := namespaceFixture(t, declaration)
		root := NewRootCommand(a)
		old, _, err := root.Find([]string{"diff"})
		require.NoError(t, err)
		root.RemoveCommand(old)
		root.AddCommand(newDiffCommand(a, func(_ string) (diffHelm, error) { return h, nil }))
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetArgs([]string{"diff", "example", "-b", backbone, "--charts-dir", dir})
		require.NoError(t, root.Execute())
		assert.Empty(t, cs.Actions(), "preview must not read or mutate namespaces")
		if declaration == "" {
			assert.Equal(t, "no differences\n", out.String())
		} else {
			assert.Equal(t, "# Declared namespaces reconciled before apply (desired configuration; not a live namespace diff)\n"+
				"---\napiVersion: v1\nkind: Namespace\nmetadata:\n    labels:\n        env: dev\n    name: one\n"+
				"---\napiVersion: v1\nkind: Namespace\nmetadata:\n    name: two\n"+
				"# Helm chart manifest diff\nno chart manifest differences\n", out.String())
		}
	}
}

func TestNamespacesDoNotChangeOfflineRender(t *testing.T) {
	for _, declaration := range []string{"", declaredNamespaces} {
		a := newState(t) // no Kubernetes client or kubeconfig
		dir := t.TempDir()
		backbone := write(t, filepath.Join(dir, "backbone.yaml"), declaration+"charts: {example: {namespace: unmanaged}}")
		write(t, filepath.Join(dir, "example", "values.yaml"), "replicas: 1\n")
		output := filepath.Join(dir, "rendered.yaml")
		root := NewRootCommand(a)
		root.SetArgs([]string{"render", "example", "-b", backbone, "--charts-dir", dir, "-o", output})
		require.NoError(t, root.Execute())
		got, err := os.ReadFile(output)
		require.NoError(t, err)
		assert.Equal(t, "replicas: 1\n", string(got))
		assert.Nil(t, a.Kube)
	}
}

func TestOtherCommandsDoNotReconcileNamespaces(t *testing.T) {
	for _, args := range [][]string{{"history", "example"}, {"logs", "example"}, {"exec", "example", "true"}, {"rollback", "example", "1", "-y"}} {
		t.Run(args[0], func(t *testing.T) {
			a, cs, _, backbone, _ := namespaceFixture(t, declaredNamespaces)
			cs.PrependReactor("*", "namespaces", func(_ clienttesting.Action) (bool, runtime.Object, error) {
				t.Error("command must not access namespaces")
				return true, nil, errors.New("unexpected namespace access")
			})
			root := NewRootCommand(a)
			root.SetArgs(append(args, "-b", backbone))
			// The test has no release or real kubeconfig; reaching the usual read path
			// must fail without first attempting namespace reconciliation.
			require.Error(t, root.Execute())
			for _, action := range cs.Actions() {
				assert.NotEqual(t, "namespaces", action.GetResource().Resource)
			}
		})
	}
}
