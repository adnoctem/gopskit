package fs

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTempDir(t *testing.T) {
	asrt := assert.New(t)

	path, err := TempDir("gopskit-test-*")
	asrt.NoError(err)
	defer func() { _ = os.RemoveAll(path) }()

	asrt.True(CheckIfExists(path))
	asrt.True(strings.Contains(path, "gopskit-test-"))
}

func TestTempFile(t *testing.T) {
	asrt := assert.New(t)

	f, err := TempFile("gopskit-test-*")
	asrt.NoError(err)
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = f.Close() }()

	asrt.True(CheckIfExists(f.Name()))
}
