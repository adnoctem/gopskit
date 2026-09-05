package proc

import (
	stderrs "errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/fmjstudios/gopskit/pkg/log"
)

// Must ensures the return values of a function are not errors therefore negating
// any further checks that have to be made. Most commonly used as a wrapper within
// new object declarations
func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// LookPath is a safer alternative to the direct use of exec.LookPath, which
// mitigates issues caused by non-nil errors due to the fact that the Go runtime
// resolved the executable to the current directory instead of a generic location
// in $PATH
func LookPath(path string) (string, error) {
	path, err := exec.LookPath(path)
	if stderrs.Is(err, exec.ErrDot) {
		return path, nil
	}

	return path, err
}

type cleanupFunc func() int

func AwaitCancel(cleanup cleanupFunc) {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(c)

	log.Global.Infof("Received signal: %s. Shutting down gracefully...", <-c)
	os.Exit(cleanup())
}

func AwaitCancelWithChannel(cleanup cleanupFunc, signalChan chan os.Signal) {
	if cap(signalChan) < 2 {
		log.Global.Fatal("signalChan must have a minimum capacity of 2 elements!")
	}

	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signalChan)

	log.Global.Infof("Received signal: %s. Shutting down gracefully...", <-signalChan)
	os.Exit(cleanup())
}
