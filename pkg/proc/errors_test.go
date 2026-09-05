package proc

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecuteErrorError(t *testing.T) {
	asrt := assert.New(t)

	err := ExecuteError{ExitCode: 1, Err: errors.New("boom")}
	asrt.Equal("exited with code: 1. error: boom", err.Error())
}

func TestNotInPathErrorError(t *testing.T) {
	asrt := assert.New(t)

	err := NotInPathError{Executable: "does-not-exist"}
	asrt.Equal("executable: does-not-exist was not found in system PATH", err.Error())
}
