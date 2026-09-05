package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGlobalValuesAppliesTo(t *testing.T) {
	asrt := assert.New(t)

	wildcard := GlobalValues{Applies: []string{"*"}}
	asrt.True(wildcard.AppliesTo("anything"))

	scoped := GlobalValues{Applies: []string{"vault", "keycloak"}}
	asrt.True(scoped.AppliesTo("vault"))
	asrt.False(scoped.AppliesTo("other"))

	empty := GlobalValues{}
	asrt.False(empty.AppliesTo("vault"))
}

func TestHookActionValidate(t *testing.T) {
	asrt := assert.New(t)

	asrt.NoError(HookAction{Local: "script.sh"}.Validate())
	asrt.NoError(HookAction{Exec: "echo hi"}.Validate())
	asrt.Error(HookAction{}.Validate(), "neither set")
	asrt.Error(HookAction{Local: "script.sh", Exec: "echo hi"}.Validate(), "both set")
}

func TestConfigGlobalsFor(t *testing.T) {
	asrt := assert.New(t)

	cfg := &Config{
		Globals: []GlobalValues{
			{Path: "/a.yaml", Applies: []string{"*"}},
			{Path: "/b.yaml", Applies: []string{"vault"}},
			{Path: "/c.yaml", Applies: []string{"keycloak"}},
		},
	}

	asrt.Equal([]string{"/a.yaml", "/b.yaml"}, cfg.GlobalsFor("vault"))
	asrt.Equal([]string{"/a.yaml"}, cfg.GlobalsFor("unrelated"))
}

func TestLoad(t *testing.T) {
	t.Run("parses charts and globals", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		path := filepath.Join(dir, "backbone.yaml")
		asrt.NoError(os.WriteFile(path, []byte(`
globals:
  - path: globals.yaml
    applies: ["*"]
charts:
  vault:
    version: "1.2.3"
    repository: "https://charts.example.com"
    namespace: vault
`), 0600))

		cfg, err := Load(path)
		asrt.NoError(err)
		asrt.Len(cfg.Globals, 1)
		asrt.Equal("1.2.3", cfg.Charts["vault"].Version)
		asrt.Equal("vault", cfg.Charts["vault"].Namespace)
	})

	t.Run("resolves relative global paths against the config file's own directory", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		path := filepath.Join(dir, "sub", "backbone.yaml")
		asrt.NoError(os.MkdirAll(filepath.Join(dir, "sub"), 0755))
		asrt.NoError(os.WriteFile(path, []byte(`
globals:
  - path: globals.yaml
    applies: ["*"]
  - path: /already/absolute.yaml
    applies: ["*"]
charts: {}
`), 0600))

		cfg, err := Load(path)
		asrt.NoError(err)
		asrt.Equal(filepath.Join(dir, "sub", "globals.yaml"), cfg.Globals[0].Path)
		asrt.Equal("/already/absolute.yaml", cfg.Globals[1].Path)
	})

	t.Run("rejects a hook action with an invalid combination of fields", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		path := filepath.Join(dir, "backbone.yaml")
		asrt.NoError(os.WriteFile(path, []byte(`
charts:
  vault:
    version: "1.0.0"
    hooks:
      pre-apply:
        - local: ""
          exec: ""
`), 0600))

		_, err := Load(path)
		asrt.Error(err)
	})

	t.Run("errors on a missing file", func(t *testing.T) {
		asrt := assert.New(t)

		_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
		asrt.Error(err)
	})

	t.Run("errors on invalid YAML", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		path := filepath.Join(dir, "backbone.yaml")
		asrt.NoError(os.WriteFile(path, []byte("charts: [not a map"), 0600))

		_, err := Load(path)
		asrt.Error(err)
	})
}
