package subprocess_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/subprocess"
)

func TestMain(m *testing.M) {
	clitest.ServeChild()
	os.Exit(m.Run())
}

func run(o subprocess.Options, steps ...string) subprocess.Result {
	argv := clitest.ChildCommand(steps...)
	o.Name, o.Args = argv[0], argv[1:]
	return subprocess.Run(o)
}

func TestRunCopiesOutputAndExitCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	started := false
	res := run(subprocess.Options{
		Stdin: strings.NewReader("from stdin\n"), Stdout: &stdout, Stderr: &stderr,
		Started: func() { started = true },
	}, "out:hello", "err:oops", "stdin", "exit:3")
	if res.ExitCode != 3 || res.StartErr != nil || !started {
		t.Errorf("result = %+v, started = %v; want exit 3", res, started)
	}
	if stdout.String() != "hello\nfrom stdin\n" || stderr.String() != "oops\n" {
		t.Errorf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRunReportsACommandThatCannotStart(t *testing.T) {
	notExecutable := filepath.Join(t.TempDir(), "backup.sh")
	if err := os.WriteFile(notExecutable, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		want int
	}{
		{"chronos-no-such-command", 127},
		{filepath.Join(t.TempDir(), "missing.sh"), 127},
		{notExecutable, 126},
	}
	for _, tt := range tests {
		t.Run(filepath.Base(tt.name), func(t *testing.T) {
			started := false
			res := subprocess.Run(subprocess.Options{Name: tt.name, Started: func() { started = true }})
			if res.ExitCode != tt.want || res.StartErr == nil || started {
				t.Errorf("result = %+v, started = %v; want exit %d and a start error", res, started, tt.want)
			}
			if _, ok := errors.AsType[*exec.Error](res.StartErr); ok || strings.Contains(res.StartErr.Error(), tt.name) {
				t.Errorf("StartErr = %q repeats the command's name", res.StartErr)
			}
		})
	}
}
