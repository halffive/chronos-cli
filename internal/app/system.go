package app

import (
	"context"
	"os"
	"time"

	"golang.org/x/term"
)

func NewSystemFactory() *Factory {
	return &Factory{IO: systemIO(), Getenv: os.Getenv, Now: time.Now}
}

func systemIO() *IOStreams {
	inFd, outFd := int(os.Stdin.Fd()), int(os.Stdout.Fd())
	streams := &IOStreams{
		In:         os.Stdin,
		Out:        os.Stdout,
		ErrOut:     os.Stderr,
		InTTY:      term.IsTerminal(inFd),
		OutTTY:     term.IsTerminal(outFd),
		ReadSecret: func(ctx context.Context) (string, error) { return readSecret(ctx, inFd) },
	}
	if width, _, err := term.GetSize(outFd); err == nil {
		streams.Width = width
	}
	return streams
}

// readSecret reads without echo. If ctx ends first it restores the terminal,
// since the abandoned read would otherwise leave echo off.
func readSecret(ctx context.Context, fd int) (string, error) {
	state, err := term.GetState(fd)
	if err != nil {
		return "", err
	}
	secret, err := readWithContext(ctx, func() (string, error) {
		b, err := term.ReadPassword(fd)
		return string(b), err
	})
	if ctx.Err() != nil {
		term.Restore(fd, state)
	}
	return secret, err
}
