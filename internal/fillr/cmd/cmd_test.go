package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adnoctem/gopskit/internal/fillr/app"
	"github.com/adnoctem/gopskit/pkg/core"
	"github.com/stretchr/testify/assert"
)

func newTestFillrState() *app.State {
	return &app.State{API: &core.API{Name: app.Name}}
}

func TestFillrArgsValidation(t *testing.T) {
	t.Run("errors with no arguments", func(t *testing.T) {
		asrt := assert.New(t)

		cmd := NewRootCommand(newTestFillrState())
		cmd.SetArgs([]string{})
		asrt.Error(cmd.Execute())
	})

	t.Run("errors with more than one argument", func(t *testing.T) {
		asrt := assert.New(t)

		cmd := NewRootCommand(newTestFillrState())
		cmd.SetArgs([]string{"a.yaml", "b.yaml"})
		asrt.Error(cmd.Execute())
	})

	t.Run("errors when the given file does not exist", func(t *testing.T) {
		asrt := assert.New(t)

		cmd := NewRootCommand(newTestFillrState())
		cmd.SetArgs([]string{filepath.Join(t.TempDir(), "missing.yaml")})
		asrt.Error(cmd.Execute())
	})
}

func TestFillrRun(t *testing.T) {
	asrt := assert.New(t)

	dir := t.TempDir()
	input := filepath.Join(dir, "values.yaml")
	asrt.NoError(os.WriteFile(input, []byte("foo: bar\n"), 0600))

	output := filepath.Join(dir, "out.yaml")
	cmd := NewRootCommand(newTestFillrState())
	cmd.SetArgs([]string{input, "-o", output})
	asrt.NoError(cmd.Execute())

	got, err := os.ReadFile(output)
	asrt.NoError(err)
	asrt.Contains(string(got), `foo: '{{ .Values | get "foo" "bar" }}'`)
}
