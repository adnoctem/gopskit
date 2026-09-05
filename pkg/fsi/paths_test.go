package fs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	knownHome      = os.Getenv("HOME")
	knownConfig    = filepath.Join(knownHome, ".config")
	knownDataOrLog = filepath.Join(knownHome, ".local", "share")
)

func TestNew(t *testing.T) {
	p, _ := Paths()
	asrt := assert.New(t)

	// as long as we're in a subdirectory of the knownConfig, we succeed
	got := p.Config
	asrt.Contains(got, knownConfig, "ConfigDir matches")

	// test data path
	got = p.Data
	asrt.Contains(got, knownDataOrLog)

	// test log path
	got = p.Log
	asrt.Contains(got, knownDataOrLog)

	// test cache path
	osc, _ := os.UserCacheDir()
	got = p.Cache
	asrt.Contains(got, osc)
}

func TestPathsWithAppName(t *testing.T) {
	asrt := assert.New(t)

	p, err := Paths(WithAppName("myapp"))
	asrt.NoError(err)
	asrt.Equal("myapp", p.AppName)
	asrt.Contains(p.Config, "myapp")
}

func TestPathsWithConfigPathExistsTracking(t *testing.T) {
	asrt := assert.New(t)

	dir := t.TempDir()
	existing := filepath.Join(dir, "config.yaml")
	missing := filepath.Join(dir, "missing.yaml")
	asrt.NoError(os.WriteFile(existing, []byte("x"), 0600))

	p, err := Paths(WithConfigPath(existing, missing))
	asrt.NoError(err)

	// an existing config path must be tracked as such
	asrt.True(p.Exists[existing])
	// a genuinely missing config path must not be silently reported as existing
	asrt.False(p.Exists[missing])
}

func TestPathsWithConfigType(t *testing.T) {
	asrt := assert.New(t)

	p, err := Paths(WithConfigType("json"))
	asrt.NoError(err)
	asrt.Equal([]string{"json"}, p.ConfigTypes)
}
