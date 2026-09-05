package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func newTestExecutor(t *testing.T) *proc.Executor {
	t.Helper()

	e, err := proc.NewExecutor()
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// RunHooks only forwards the pod to kc.Exec, which none of these tests exercise (the "local" hook
// path never touches the cluster), so a zero-value Pod is enough.
var testPod = corev1.Pod{}

func TestRunHooksLocal(t *testing.T) {
	t.Run("runs a local script and continues on success", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		script := filepath.Join(dir, "hook.sh")
		asrt.NoError(os.WriteFile(script, []byte("#!/bin/sh\necho hook-ran\n"), 0755))

		err := RunHooks([]HookAction{{Local: script}}, newTestExecutor(t), nil, testPod, nil)
		asrt.NoError(err)
	})

	t.Run("stops and returns an error when a local hook fails", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		script := filepath.Join(dir, "hook.sh")
		asrt.NoError(os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0755))

		err := RunHooks([]HookAction{{Local: script}}, newTestExecutor(t), nil, testPod, nil)
		asrt.Error(err)
	})

	t.Run("rejects an invalid hook action before running anything", func(t *testing.T) {
		asrt := assert.New(t)

		err := RunHooks([]HookAction{{}}, newTestExecutor(t), nil, testPod, nil)
		asrt.Error(err)
	})

	t.Run("runs multiple hooks in order, stopping at the first failure", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		marker := filepath.Join(dir, "marker")
		first := filepath.Join(dir, "first.sh")
		asrt.NoError(os.WriteFile(first, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0755))
		second := filepath.Join(dir, "second.sh")
		asrt.NoError(os.WriteFile(second, []byte("#!/bin/sh\nexit 1\n"), 0755))
		third := filepath.Join(dir, "third.sh")
		asrt.NoError(os.WriteFile(third, []byte("#!/bin/sh\ntouch "+marker+"-third\n"), 0755))

		err := RunHooks([]HookAction{{Local: first}, {Local: second}, {Local: third}}, newTestExecutor(t), nil, testPod, nil)
		asrt.Error(err)

		_, statErr := os.Stat(marker)
		asrt.NoError(statErr, "the first hook should have run")
		_, statErr = os.Stat(marker + "-third")
		asrt.Error(statErr, "the third hook must not run after the second one failed")
	})
}
