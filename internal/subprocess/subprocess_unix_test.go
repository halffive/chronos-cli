//go:build unix

package subprocess_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"chronos.sh/cli/internal/subprocess"
)

type readyWriter struct {
	once  sync.Once
	ready chan struct{}
}

func (w *readyWriter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("ready")) {
		w.once.Do(func() { close(w.ready) })
	}
	return len(p), nil
}

func TestRunForwardsSignals(t *testing.T) {
	signals := make(chan os.Signal, 1)
	out := &readyWriter{ready: make(chan struct{})}
	go func() {
		<-out.ready
		signals <- syscall.SIGTERM
	}()
	res := run(subprocess.Options{Stdout: out, Signals: signals}, "out:ready", "sleep:10s")
	if res.ExitCode != 128+int(syscall.SIGTERM) {
		t.Errorf("exit = %d, want %d", res.ExitCode, 128+int(syscall.SIGTERM))
	}
}

func TestRunDoesNotWaitForAProcessLeftHoldingStdout(t *testing.T) {
	var stdout bytes.Buffer
	started := time.Now()
	res := run(subprocess.Options{Stdout: &stdout, WaitDelay: 100 * time.Millisecond}, "hold-stdout", "exit:4")
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Errorf("Run took %s, want about WaitDelay", elapsed)
	}
	if res.ExitCode != 4 {
		t.Errorf("exit = %d, want 4 from the command, not the wait", res.ExitCode)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("terminal went away") }

func TestRunLeavesABrokenStdoutToTheCommand(t *testing.T) {
	res := run(subprocess.Options{Stdout: brokenWriter{}, Stderr: io.Discard}, "spew", "exit:0")
	if res.ExitCode != 128+int(syscall.SIGPIPE) {
		t.Errorf("exit = %d, want %d: the command dies of SIGPIPE as it would without chronos", res.ExitCode, 128+int(syscall.SIGPIPE))
	}
}
