package helm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func writeValuesFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMergeValues(t *testing.T) {
	t.Run("later files override earlier ones (base+overlay precedence)", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		base := writeValuesFile(t, dir, "base.yaml", "replicas: 1\nimage:\n  tag: v1\n")
		overlay := writeValuesFile(t, dir, "overlay.yaml", "replicas: 3\n")

		merged, err := MergeValues(base, overlay)
		asrt.NoError(err)
		asrt.Equal(3, merged["replicas"])

		image, ok := merged["image"].(map[string]interface{})
		asrt.True(ok)
		asrt.Equal("v1", image["tag"])
	})

	t.Run("merges nested maps across multiple files without dropping keys", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		globals := writeValuesFile(t, dir, "globals.yaml", "env: prod\n")
		base := writeValuesFile(t, dir, "base.yaml", "app:\n  name: carto\n  port: 8080\n")
		overlay := writeValuesFile(t, dir, "overlay.yaml", "app:\n  port: 9090\n")

		merged, err := MergeValues(globals, base, overlay)
		asrt.NoError(err)
		asrt.Equal("prod", merged["env"])

		app, ok := merged["app"].(map[string]interface{})
		asrt.True(ok)
		asrt.Equal("carto", app["name"])
		asrt.Equal(9090, app["port"])
	})

	t.Run("no files returns an empty map", func(t *testing.T) {
		asrt := assert.New(t)

		merged, err := MergeValues()
		asrt.NoError(err)
		asrt.Empty(merged)
	})

	t.Run("errors on a missing file", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		_, err := MergeValues(filepath.Join(dir, "missing.yaml"))
		asrt.Error(err)
	})

	t.Run("errors on invalid YAML", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		bad := writeValuesFile(t, dir, "bad.yaml", "not: [valid: yaml")

		_, err := MergeValues(bad)
		asrt.Error(err)
	})
}

func TestDiffValues(t *testing.T) {
	asrt := assert.New(t)

	same := map[string]interface{}{"a": 1}
	asrt.Empty(DiffValues(same, same))

	old := map[string]interface{}{"a": 1}
	updated := map[string]interface{}{"a": 2}
	diff := DiffValues(old, updated)
	asrt.NotEmpty(diff)
	asrt.Contains(diff, "Values mismatch")
}
