package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/term"

	"chronos.sh/cli/internal/config"
)

func NewSystemFactory() *Factory {
	return &Factory{IO: systemIO(), Getenv: os.Getenv, Now: time.Now, Keychain: config.SystemKeychain{}}
}

func InterruptSignals() []os.Signal {
	signals := []os.Signal{os.Interrupt, syscall.SIGTERM}
	if signal.Ignored(syscall.SIGHUP) {
		return signals
	}
	return append(signals, syscall.SIGHUP)
}

func systemIO() *IOStreams {
	inFd, outFd := int(os.Stdin.Fd()), int(os.Stdout.Fd())
	streams := &IOStreams{
		In:         os.Stdin,
		Out:        os.Stdout,
		ErrOut:     os.Stderr,
		InTTY:      term.IsTerminal(inFd),
		OutTTY:     term.IsTerminal(outFd),
		ErrTTY:     term.IsTerminal(int(os.Stderr.Fd())),
		ReadSecret: func(ctx context.Context) (string, error) { return readSecret(ctx, inFd) },
	}
	if width, _, err := term.GetSize(outFd); err == nil {
		streams.Width = width
	}
	return streams
}

func readSecret(ctx context.Context, fd int) (string, error) {
	state, err := term.GetState(fd)
	if err != nil {
		return "", err
	}
	return readHidden(ctx, hiddenRead{
		read: func() (string, error) {
			b, err := term.ReadPassword(fd)
			return string(b), err
		},
		restore: func() { term.Restore(fd, state) },
	})
}

type hiddenRead struct {
	read    func() (string, error)
	restore func()
}

// readHidden turns Ctrl-C, Ctrl-\, SIGTERM and SIGHUP into ctx ending, and
// restores the terminal when ctx ends, since the abandoned read would leave
// echo off.
func readHidden(ctx context.Context, r hiddenRead) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	ctx, stop := signal.NotifyContext(ctx, append(InterruptSignals(), syscall.SIGQUIT)...)
	defer stop()
	secret, err := ReadWithContext(ctx, r.read)
	if ctx.Err() != nil {
		r.restore()
	}
	return secret, err
}
