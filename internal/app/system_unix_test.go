//go:build unix

package app_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/clitest"
)

const (
	wrapperFlag        = "-chronos-test-wrapper"
	reportTerminalFlag = "-chronos-test-report-terminal"
)

func TestMain(m *testing.M) {
	clitest.ServeChild()
	if len(os.Args) > 1 && os.Args[1] == wrapperFlag {
		os.Exit(wrapChild())
	}
	if len(os.Args) > 1 && os.Args[1] == reportTerminalFlag {
		fmt.Println(app.NewSystemFactory().IO.HasControllingTerminal)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// wrapChild catches signals as chronos monitors exec does, then runs a child
// and reports how it ended.
func wrapChild() int {
	signal.Notify(make(chan os.Signal, 1), app.InterruptSignals()...)
	signal.Notify(make(chan os.Signal, 1), app.ForwardSignals()...)
	argv := clitest.ChildCommand("out:ready", "sleep:500ms", "out:survived")
	child := exec.Command(argv[0], argv[1:]...)
	child.Stdout = os.Stdout
	fmt.Println("child:", child.Run())
	return 0
}

func TestInterruptSignalsLeaveAnIgnoredHangupIgnored(t *testing.T) {
	t.Cleanup(func() { signal.Reset(syscall.SIGHUP) })
	signal.Notify(make(chan os.Signal, 1), syscall.SIGHUP)
	if !slices.Contains(app.InterruptSignals(), os.Signal(syscall.SIGHUP)) {
		t.Error("SIGHUP is missing while not ignored")
	}
	signal.Ignore(syscall.SIGHUP)
	if slices.Contains(app.InterruptSignals(), os.Signal(syscall.SIGHUP)) {
		t.Error("nohup would stop protecting the run: SIGHUP is listed while ignored")
	}
}

func TestInterruptSignalsLeaveAnIgnoredInterruptIgnored(t *testing.T) {
	t.Cleanup(func() { signal.Reset(syscall.SIGINT) })
	signal.Notify(make(chan os.Signal, 1), syscall.SIGINT)
	if !slices.Contains(app.InterruptSignals(), os.Signal(syscall.SIGINT)) {
		t.Error("SIGINT is missing while not ignored")
	}
	signal.Ignore(syscall.SIGINT)
	if slices.Contains(app.InterruptSignals(), os.Signal(syscall.SIGINT)) {
		t.Error("a script's background job would stop ignoring Ctrl-C: SIGINT is listed while ignored")
	}
}

func TestACommandRunUnderNohupKeepsIgnoringHangupAndInterrupt(t *testing.T) {
	wrapper := exec.Command("sh", "-c", `trap "" HUP INT; exec "$0" "$1"`, os.Args[0], wrapperFlag)
	wrapper.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := wrapper.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := wrapper.Start(); err != nil {
		t.Fatal(err)
	}
	lines := bufio.NewScanner(stdout)
	var got []string
	for lines.Scan() {
		got = append(got, lines.Text())
		if lines.Text() == "ready" {
			syscall.Kill(-wrapper.Process.Pid, syscall.SIGHUP)
			syscall.Kill(-wrapper.Process.Pid, syscall.SIGINT)
		}
	}
	wrapper.Wait()
	if want := []string{"ready", "survived", "child: <nil>"}; !slices.Equal(got, want) {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestNoControllingTerminalInANewSession(t *testing.T) {
	cmd := exec.Command(os.Args[0], reportTerminalFlag)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "false" {
		t.Errorf("HasControllingTerminal = %q, %v; want false without a terminal", out, err)
	}
}

func TestHiddenReadKeepsAnIgnoredHangupIgnored(t *testing.T) {
	signal.Ignore(syscall.SIGHUP)
	t.Cleanup(func() { signal.Reset(syscall.SIGHUP) })
	secret, err := app.ReadHidden(t.Context(), func() (string, error) {
		syscall.Kill(os.Getpid(), syscall.SIGHUP)
		time.Sleep(100 * time.Millisecond)
		return "key", nil
	}, func() {})
	if secret != "key" || err != nil {
		t.Errorf("secret = %q, err = %v; want the read to finish", secret, err)
	}
}

func TestHiddenReadDoesNothingWhenAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	restored := false
	_, err := app.ReadHidden(ctx, func() (string, error) { return "", nil }, func() { restored = true })
	if !errors.Is(err, context.Canceled) || restored {
		t.Errorf("err = %v, restored = %v; want context.Canceled before the read starts", err, restored)
	}
}

func TestHiddenReadRestoresTheTerminalOnASignal(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			notified := make(chan os.Signal, 1)
			signal.Notify(notified, sig)
			defer signal.Stop(notified)
			started := make(chan struct{})
			release := make(chan struct{})
			defer close(release)
			go func() {
				<-started
				syscall.Kill(os.Getpid(), sig)
			}()
			restored := false
			_, err := app.ReadHidden(t.Context(), func() (string, error) {
				close(started)
				<-release
				return "", nil
			}, func() { restored = true })
			if !errors.Is(err, context.Canceled) || !restored {
				t.Errorf("err = %v, restored = %v; want context.Canceled and the terminal restored", err, restored)
			}
		})
	}
}
