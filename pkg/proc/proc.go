// Package proc implements a generic Shell process executor, which allows us to execute
// arbitrary commands from within the program.
package proc

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
)

// Opt is a configuration option for the initialization of new Executor's
type Opt func(e *Executor) error

// ExecuteOpt is a runtime option for the Executor's Execute methods
type ExecuteOpt func(e *Executor)

type Executor struct {
	// inheritEnv determines whether the new process should inherit the environment
	// of the starting process
	inheritEnv bool

	// writers are implementations of the io.WriteCloser interface which the
	// command will output the standard streams to. The first element will be
	// standard output, and the second will be standard error. Standard input is
	// omitted since we're looking to execute programs from within an application.
	//
	// NOTE: This value is reset on every call of the Execute method, requiring
	// re-configuration via the ExecutorOpt array.
	writers []io.Writer

	// outputs is an array of file paths we will write the standard output of the
	// started process to. Regardless of how many files there are, all of them will
	// be filled with the standard OUTPUT of the command.
	//
	// NOTE: This value is reset on every call of the Execute method, requiring
	// re-configuration via the ExecutorOpt array.
	outputs []string

	// lock is a Mutex which ensures that only one goroutine may modify the configuration
	// lock sync.Mutex

	// exec.Cmd is the embedded Go-native Command to execute
	*exec.Cmd
}

// NewExecutor creates a new Executor with the provided configuration options.
// The method takes in a set of configuration Options which may configure the
// Executor. These Options allow Execute to do things like write files,
// byte-buffers and e.g. inherit the host's environment configuration.
func NewExecutor(opts ...Opt) (*Executor, error) {
	e := DefaultExecutor()

	// (re)-configure
	for _, o := range opts {
		if err := o(e); err != nil {
			return nil, err
		}
	}

	return e, nil
}

// DefaultExecutor is the default Executor implementation and the base for
// latter configurations via ExecutorOpt's
func DefaultExecutor() *Executor {
	return &Executor{
		inheritEnv: false,
	}
}

// WithInheritedEnv configures the Execute function to inherit the current OS environment
// settings into the command to be executed
func WithInheritedEnv() Opt {
	return func(e *Executor) error {
		e.inheritEnv = true
		return nil
	}
}

// WithMultiWriters configures the Execute function to simultaneously write to both the system's
// StdOut/Err and to the given byte-buffers, in order (StdOut first, StdErr second). Buffers must
// be passed by pointer so the caller can read them back after Execute returns.
func WithMultiWriters(writers ...*bytes.Buffer) ExecuteOpt {
	return func(e *Executor) {
		var stdout, stderr io.Writer = os.Stdout, os.Stderr

		if len(writers) > 0 && writers[0] != nil {
			stdout = io.MultiWriter(os.Stdout, writers[0])
		}
		if len(writers) > 1 && writers[1] != nil {
			stderr = io.MultiWriter(os.Stderr, writers[1])
		}

		e.writers = append(e.writers, stdout, stderr)
	}
}

// WithWriters configures the Execute function to fill the given byte-buffers with the StdOut and
// StdErr output respectively, in order. Buffers must be passed by pointer so the caller can read
// them back after Execute returns.
func WithWriters(writers ...*bytes.Buffer) ExecuteOpt {
	return func(e *Executor) {
		var stdout, stderr io.Writer

		if len(writers) > 0 {
			stdout = writers[0]
		}
		if len(writers) > 1 {
			stderr = writers[1]
		}

		e.writers = append(e.writers, stdout, stderr)
	}
}

// WithOutputs configures the Execute function to spawn a GoRoutine which writes
// the standard output of the command to a file on the filesystem
func WithOutputs(paths ...string) ExecuteOpt {
	return func(e *Executor) {
		e.outputs = paths
	}
}

// Execute executes a system command on the host operating system's shell, however avoids Shell
// expansions like globs etc. The method takes in a set of arguments and a set of configuration
// Options which may configure the execution of the method.
// These Options allow Execute to do things like write files, byte-buffers and e.g. inherit the
// host's environment configuration.
func (e *Executor) Execute(args []string, opts ...ExecuteOpt) ([]string, error) {
	ctx, ctxCancel := context.WithCancel(context.TODO())
	defer ctxCancel()

	e.Cmd = exec.CommandContext(ctx, args[0], args[1:]...)

	// sanity
	if _, err := LookPath(args[0]); err != nil {
		return nil, NotInPathError{Executable: args[0]}
	}

	// update values for each call
	if err := e.configure(); err != nil {
		return nil, err
	}

	// (re)-configure
	for _, o := range opts {
		o(e)
	}

	// create pipes
	readers := []io.ReadCloser{Must(e.StdoutPipe()), Must(e.StderrPipe())}

	// execute command
	if err := e.Start(); err != nil {
		return nil, err
	}

	// Each pipe is a one-shot stream, so it must be read exactly once. We capture stdout/stderr
	// into their own buffers (fanning out to any configured e.writers along the way) concurrently,
	// since sequentially draining stdout then stderr risks a deadlock if the process fills the
	// unread pipe's OS buffer while we're still blocked reading the other one.
	var stdoutBuf, stderrBuf bytes.Buffer
	dst := []io.Writer{&stdoutBuf, &stderrBuf}
	for i, wr := range e.writers {
		if wr != nil {
			dst[i] = io.MultiWriter(dst[i], wr)
		}
	}

	var wg sync.WaitGroup
	copyErrs := make([]error, 2)
	wg.Add(2)
	for i := range readers {
		go func(i int) {
			defer wg.Done()
			_, copyErrs[i] = copyBytes(readers[i], dst[i])
		}(i)
	}
	wg.Wait()

	for _, err := range copyErrs {
		if err != nil {
			return nil, err
		}
	}

	// write stdout to files (if set)
	for _, out := range e.outputs {
		if err := os.WriteFile(out, stdoutBuf.Bytes(), 0644); err != nil {
			return nil, err
		}
	}

	output := []string{stdoutBuf.String(), stderrBuf.String()}

	// wait for command to finish
	if err := e.Wait(); err != nil {
		return nil, ExecuteError{ExitCode: e.ProcessState.ExitCode(), Err: err}
	}

	return output, nil
}

// configure configures the Executor
func (e *Executor) configure() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	e.Dir = wd

	if e.inheritEnv {
		e.Env = append(e.Env, os.Environ()...)
	}

	e.Stdin = nil
	e.writers = []io.Writer{}
	e.outputs = []string{}

	return nil
}

// copyBytes copies bytes from an io.Reader into an io.Writer and returns the entire
// buffers as a whole as well as an error
func copyBytes(src io.Reader, dst io.Writer) ([]byte, error) {
	var out []byte
	buf := make([]byte, 1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			d := buf[:n]
			out = append(out, d...)
			_, err := dst.Write(d)
			if err != nil {
				return out, err
			}
		}
		if err != nil {
			// Read returns io.EOF at the end of file, which is not an error for us
			if err == io.EOF {
				err = nil
			}
			return out, err
		}
	}
}
