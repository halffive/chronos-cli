// Package app holds what commands share that does not depend on the
// command-line parser: the SDK client, output mode, dry runs, prompts and
// waiting for jobs.
package app

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	chronos "chronos.sh/go"

	"chronos.sh/cli/internal/build"
	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/output"
	"chronos.sh/cli/internal/transport"
)

// IOStreams are the process's standard streams and what is known about them.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
	InTTY  bool
	OutTTY bool
	ErrTTY bool
	// Width is the terminal width of Out, or 0.
	Width int
	// ReadSecret reads one line from the terminal without echoing it.
	ReadSecret func(ctx context.Context) (string, error)
}

// Factory holds the state of one run of the CLI.
type Factory struct {
	IO     *IOStreams
	Getenv config.Env
	Now    func() time.Time
	// ClientOptions are appended to every client's options.
	ClientOptions []chronos.Option
	// JobPollInterval is the time between checks while waiting for a job.
	// Zero is the SDK's default.
	JobPollInterval time.Duration
	// Keychain holds saved API keys. Nil means the machine has no keychain,
	// so a Factory built by hand never reaches the system one.
	Keychain config.Keychain

	JSON    bool
	BaseURL string
	DryRun  bool

	recorder    *transport.Recorder
	credentials *config.Credentials
}

func (f *Factory) Printer() *output.Printer {
	return &output.Printer{
		Out:    f.IO.Out,
		Err:    f.IO.ErrOut,
		JSON:   f.JSON,
		TTY:    f.IO.OutTTY,
		ErrTTY: f.IO.ErrTTY,
		Color:  f.IO.OutTTY && f.Getenv("NO_COLOR") == "" && f.Getenv("TERM") != "dumb",
		Width:  f.IO.Width,
		Now:    f.Now,
	}
}

func (f *Factory) ConfigDir() (string, error) {
	return config.Dir(f.Getenv)
}

func (f *Factory) Config() (*config.File, error) {
	dir, err := f.ConfigDir()
	if err != nil {
		return nil, err
	}
	return config.Load(dir)
}

// Credentials resolves the key and base URL once per run, so the keychain
// prompts at most once. The config file is not read when CHRONOS_API_KEY is
// set, so a broken file cannot block an env key.
func (f *Factory) Credentials() (config.Credentials, error) {
	if f.credentials != nil {
		return *f.credentials, nil
	}
	file := config.Empty()
	if config.EnvAPIKey(f.Getenv) == "" {
		var err error
		if file, err = f.Config(); err != nil {
			return config.Credentials{}, err
		}
	}
	creds := file.Resolve(f.Getenv, f.BaseURL)
	if !f.DryRun {
		var err error
		if creds, err = config.LoadSecret(f.Keychain, creds); err != nil {
			return config.Credentials{}, err
		}
	}
	f.credentials = &creds
	return creds, nil
}

// dryRunKey stands in for a missing key under --dry-run, which sends nothing.
const dryRunKey = "chrns_dry_run"

func (f *Factory) Client() (*chronos.Client, error) {
	creds, err := f.Credentials()
	if err != nil {
		return nil, err
	}
	switch {
	case strings.ContainsFunc(creds.APIKey, unicode.IsControl):
		return nil, controlCharactersError(creds.Source)
	case creds.APIKey != "":
	case f.DryRun:
		creds.APIKey = dryRunKey
	case creds.SavedFor != "":
		return nil, exit.MissingKeyf("Your saved key is for %s, not %s. Set CHRONOS_API_KEY to use a key with %s.", creds.SavedFor, creds.BaseURL, creds.BaseURL)
	default:
		return nil, exit.MissingKeyf("No API key found. Set CHRONOS_API_KEY, or run chronos auth login.")
	}
	return f.ClientWith(creds)
}

func controlCharactersError(source config.Source) error {
	if source == config.SourceEnv {
		return exit.MissingKeyf("The API key in CHRONOS_API_KEY contains control characters. Copy it again from the dashboard.")
	}
	return exit.MissingKeyf("Your saved key contains control characters. Run chronos auth login with a key copied again from the dashboard.")
}

// ClientWith returns a client for creds. All clients in a run share one
// recorder, so Sent covers every request.
func (f *Factory) ClientWith(creds config.Credentials) (*chronos.Client, error) {
	if err := config.CheckBaseURL(creds.BaseURL); err != nil {
		return nil, err
	}
	if f.recorder == nil {
		f.recorder = &transport.Recorder{}
	}
	f.recorder.DryRun = f.DryRun
	opts := append([]chronos.Option{
		chronos.WithBaseURL(creds.BaseURL),
		chronos.WithHTTPClient(&http.Client{Transport: f.recorder}),
		chronos.WithUserAgent("chronos-cli/" + build.Current().Version),
	}, f.ClientOptions...)
	if f.DryRun {
		// The SDK retries a failed GET, and a captured request is a failure.
		opts = append(opts, chronos.WithMaxRequestRetries(0))
	}
	c, err := chronos.NewClient(creds.APIKey, opts...)
	if err != nil {
		return nil, exit.Usagef("%s", exit.Sentence(strings.TrimPrefix(err.Error(), "chronos: ")))
	}
	return c, nil
}

// Sent reports what this run has sent, for classifying a failure.
func (f *Factory) Sent() exit.Sent {
	if f.recorder == nil {
		return exit.Sent{}
	}
	return exit.Sent{LastMethod: f.recorder.LastMethod(), WriteApplied: f.recorder.WriteApplied()}
}
