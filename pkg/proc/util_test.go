package proc

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMust(t *testing.T) {
	asrt := assert.New(t)

	asrt.Equal(42, Must(42, nil))
	asrt.Panics(func() { Must(0, errors.New("boom")) })
}

func TestLookPath(t *testing.T) {
	asrt := assert.New(t)

	// "ls" is expected to be present on any Unix CI runner/dev box this repo builds on
	path, err := LookPath("ls")
	asrt.NoError(err)
	asrt.NotEmpty(path)

	_, err = LookPath("definitely-not-a-real-executable-xyz")
	asrt.Error(err)
}
