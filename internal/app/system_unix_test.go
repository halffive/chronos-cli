//go:build unix

package app_test

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"testing"
	"time"

	"chronos.sh/cli/internal/app"
)

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
