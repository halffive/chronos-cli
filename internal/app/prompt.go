package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/output"
)

// WillPrompt reports whether Confirm asks its question.
func (f *Factory) WillPrompt(yes bool) bool {
	return !yes && !f.DryRun && !f.JSON && f.IO.InTTY
}

// Confirm asks a yes-or-no question on a terminal. yes is the --yes flag,
// which skips the question, as does a dry run. With --json or without a
// terminal it is a usage error, so scripts and agents never hang on a prompt.
func (f *Factory) Confirm(ctx context.Context, question string, yes bool) error {
	question = output.CleanLine(question)
	if yes || f.DryRun {
		return nil
	}
	if !f.WillPrompt(yes) {
		return exit.Usagef("%s Pass --yes to confirm. chronos prompts only in a terminal and never with --json.", question)
	}
	fmt.Fprintf(f.IO.ErrOut, "%s [y/N] ", question)
	answer, err := ReadWithContext(ctx, func() (string, error) {
		line, err := bufio.NewReader(f.IO.In).ReadString('\n')
		if errors.Is(err, io.EOF) {
			return line, nil
		}
		return line, err
	})
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	}
	return &exit.Error{Exit: exit.Failure, Code: exit.CodeCancelled, Message: "Cancelled. Nothing was changed."}
}

// ReadWithContext runs read, returning early when ctx ends so Ctrl-C works
// while reading stdin. The abandoned read finishes or dies with the process.
func ReadWithContext(ctx context.Context, read func() (string, error)) (string, error) {
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		text, err := read()
		done <- result{text, err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-done:
		return r.text, r.err
	}
}
