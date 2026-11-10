// Package subprocess runs a command for the user. It copies the command's
// output where it is told, sends it the signals it is given, and reports how
// it ended the way a shell does.
package subprocess

import (
	"cmp"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"sync"
	"time"
)

// DefaultWaitDelay is how long Run waits for the command's output to close
// after the command exits.
const DefaultWaitDelay = 5 * time.Second

type Options struct {
	Name string
	Args []string
	// Stdin, Stdout and Stderr that are *os.File are handed to the command
	// as they are, so a terminal stays a terminal. Others go through a pipe.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Signals are sent to the command while it runs.
	Signals <-chan os.Signal
	// Started, if set, is called once the command is running.
	Started func()
	// WaitDelay bounds the wait for the command's output to close after it
	// exits, since a process it left running can hold it open. Zero is
	// DefaultWaitDelay.
	WaitDelay time.Duration
}

type Result struct {
	// ExitCode is the command's exit status as a shell reports it. That is
	// 128+n when signal n killed it on Unix, 127 when it was not found and
	// 126 when it could not start.
	ExitCode int
	// StartErr is why the command could not start, or nil.
	StartErr error
}

func Run(o Options) Result {
	cmd := exec.Command(o.Name, o.Args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = o.Stdin, o.Stdout, o.Stderr
	cmd.WaitDelay = cmp.Or(o.WaitDelay, DefaultWaitDelay)
	if err := cmd.Start(); err != nil {
		return startFailure(err)
	}
	if o.Started != nil {
		o.Started()
	}
	exited := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() { forward(cmd.Process, o.Signals, exited) })
	// Wait also fails when copying the output fails or outlasts WaitDelay;
	// the exit status still comes from ProcessState.
	_ = cmd.Wait()
	close(exited)
	wg.Wait()
	if cmd.ProcessState == nil {
		return Result{ExitCode: 1}
	}
	return Result{ExitCode: exitCode(cmd.ProcessState)}
}

func forward(p *os.Process, signals <-chan os.Signal, exited <-chan struct{}) {
	for {
		select {
		case sig := <-signals:
			_ = p.Signal(sig)
		case <-exited:
			return
		}
	}
}

func startFailure(err error) Result {
	code := 126
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
		code = 127
	}
	if execErr, ok := errors.AsType[*exec.Error](err); ok {
		err = execErr.Err
	}
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		err = pathErr.Err
	}
	return Result{ExitCode: code, StartErr: err}
}
