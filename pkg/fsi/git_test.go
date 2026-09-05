package fs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// makeFakeGitRepo creates <dir>/.git with the marker files/dirs findGitMarkers
// looks for, so it's recognized as a Git root without a real repo.
func makeFakeGitRepo(t *testing.T, dir string) string {
	t.Helper()

	gitDir := filepath.Join(dir, KnownGitDir)
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, marker := range KnownGitMarkers {
		if marker == "objects" || marker == "refs" || marker == "branches" {
			if err := os.MkdirAll(filepath.Join(gitDir, marker), 0755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(filepath.Join(gitDir, marker), []byte(""), 0600); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

func TestParseGitRoot(t *testing.T) {
	t.Run("finds the root from within the .git directory's parent", func(t *testing.T) {
		asrt := assert.New(t)

		root := t.TempDir()
		makeFakeGitRepo(t, root)

		got, err := ParseGitRoot(root)
		asrt.NoError(err)
		asrt.Equal(root, got)
	})

	t.Run("walks up from a nested subdirectory to find the root", func(t *testing.T) {
		asrt := assert.New(t)

		root := t.TempDir()
		makeFakeGitRepo(t, root)

		nested := filepath.Join(root, "a", "b", "c")
		asrt.NoError(os.MkdirAll(nested, 0755))

		got, err := ParseGitRoot(nested)
		asrt.NoError(err)
		asrt.Equal(root, got)
	})

	t.Run("recognizes a bare repository (markers directly in path)", func(t *testing.T) {
		asrt := assert.New(t)

		root := t.TempDir()
		for _, marker := range KnownGitMarkers {
			if marker == "objects" || marker == "refs" || marker == "branches" {
				asrt.NoError(os.MkdirAll(filepath.Join(root, marker), 0755))
				continue
			}
			asrt.NoError(os.WriteFile(filepath.Join(root, marker), []byte(""), 0600))
		}

		got, err := ParseGitRoot(root)
		asrt.NoError(err)
		asrt.Equal(root, got)
	})

	t.Run("errors when no .git can be found up to the filesystem root", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		_, err := ParseGitRoot(dir)
		asrt.Error(err)
	})

	t.Run("errors when .git exists but is a file, not a directory", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		asrt.NoError(os.WriteFile(filepath.Join(dir, KnownGitDir), []byte(""), 0600))

		_, err := ParseGitRoot(dir)
		asrt.Error(err)
	})
}
