package fs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckIfExists(t *testing.T) {
	asrt := assert.New(t)

	dir := t.TempDir()
	existing := filepath.Join(dir, "exists.txt")
	asrt.NoError(os.WriteFile(existing, []byte("x"), 0600))

	asrt.True(CheckIfExists(existing))
	asrt.True(CheckIfExists(dir))
	asrt.False(CheckIfExists(filepath.Join(dir, "missing.txt")))
}

func TestRemove(t *testing.T) {
	t.Run("removes an existing directory tree", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		path := filepath.Join(dir, "sub")
		asrt.NoError(os.MkdirAll(path, 0755))
		asrt.NoError(os.WriteFile(filepath.Join(path, "file.txt"), []byte("x"), 0600))

		asrt.NoError(Remove(path))
		asrt.False(CheckIfExists(path))
	})

	t.Run("exits silently for a non-existing path", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		asrt.NoError(Remove(filepath.Join(dir, "does-not-exist")))
	})
}
