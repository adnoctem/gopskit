package proc

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCopyBytes(t *testing.T) {
	asrt := assert.New(t)

	src := strings.NewReader("hello world")
	var dst bytes.Buffer

	out, err := copyBytes(src, &dst)
	asrt.NoError(err)
	asrt.Equal("hello world", string(out))
	asrt.Equal("hello world", dst.String())
}

func TestNewExecutor(t *testing.T) {
	t.Run("applies options in order", func(t *testing.T) {
		asrt := assert.New(t)

		e, err := NewExecutor(WithInheritedEnv())
		asrt.NoError(err)
		asrt.True(e.inheritEnv)
	})

	t.Run("propagates an error from a failing option", func(t *testing.T) {
		asrt := assert.New(t)

		boom := errors.New("boom")
		_, err := NewExecutor(func(e *Executor) error { return boom })
		asrt.ErrorIs(err, boom)
	})
}

func TestExecute(t *testing.T) {
	t.Run("captures stdout and stderr on success", func(t *testing.T) {
		asrt := assert.New(t)

		e, err := NewExecutor()
		asrt.NoError(err)

		out, err := e.Execute([]string{"echo", "hello"})
		asrt.NoError(err)
		asrt.Contains(out[0], "hello")
		asrt.Empty(out[1])
	})

	t.Run("errors with NotInPathError for a missing executable", func(t *testing.T) {
		asrt := assert.New(t)

		e, err := NewExecutor()
		asrt.NoError(err)

		_, err = e.Execute([]string{"definitely-not-a-real-executable-xyz"})
		var notInPath NotInPathError
		asrt.ErrorAs(err, &notInPath)
	})

	t.Run("errors with ExecuteError on a non-zero exit code", func(t *testing.T) {
		asrt := assert.New(t)

		e, err := NewExecutor()
		asrt.NoError(err)

		_, err = e.Execute([]string{"false"})
		var execErr ExecuteError
		asrt.ErrorAs(err, &execErr)
		asrt.Equal(1, execErr.ExitCode)
	})

	t.Run("WithWriters fills the given buffers", func(t *testing.T) {
		asrt := assert.New(t)

		e, err := NewExecutor()
		asrt.NoError(err)

		var stdout, stderr bytes.Buffer
		_, err = e.Execute([]string{"echo", "captured"}, WithWriters(&stdout, &stderr))
		asrt.NoError(err)
		asrt.Contains(stdout.String(), "captured")
		asrt.Empty(stderr.String())
	})

	t.Run("WithOutputs writes stdout to a file", func(t *testing.T) {
		asrt := assert.New(t)

		e, err := NewExecutor()
		asrt.NoError(err)

		out := filepath.Join(t.TempDir(), "out.txt")
		_, err = e.Execute([]string{"echo", "to-file"}, WithOutputs(out))
		asrt.NoError(err)

		content, err := os.ReadFile(out)
		asrt.NoError(err)
		asrt.Contains(string(content), "to-file")
	})
}
