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
	"chronos.sh/cli/internal/oauth"
	"chronos.sh/cli/internal/output"
	"chronos.sh/cli/internal/ping"
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
	// HasControllingTerminal is false when no terminal can signal the
	// process, as under cron or systemd, and always on Windows.
	HasControllingTerminal bool
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
	// Keychain holds saved logins. Nil means the machine has no keychain,
	// so a Factory built by hand never reaches the system one.
	Keychain config.Keychain
	// OpenBrowser opens a URL in the browser. Nil means there is none, so a
	// Factory built by hand never opens the system one.
	OpenBrowser func(url string) error
	// LoginTimeout bounds the wait for the browser login. Zero is
	// DefaultLoginTimeout.
	LoginTimeout time.Duration
	// PingTimeout bounds each ping, retries included. Zero is
	// ping.DefaultTimeout.
	PingTimeout time.Duration

	JSON    bool
	BaseURL string
	DryRun  bool

	recorder    *transport.Recorder
	credentials *config.Credentials
	oauthToken  bool
}

// DefaultLoginTimeout matches how long the API keeps an authorization
// request open.
const DefaultLoginTimeout = 10 * time.Minute

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

// Credentials resolves the credential and base URL once per run, so the
// keychain prompts at most once. The config file is not read when
// CHRONOS_API_KEY is set, so a broken file cannot block an env key.
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
	case strings.ContainsFunc(creds.Secret, unicode.IsControl):
		return nil, controlCharactersError(creds.Source)
	case creds.Grant.Expired(f.Now()) && !f.DryRun:
		return nil, exit.LoginExpiredf("Your login expired on %s. Run chronos auth login to log in again, or set CHRONOS_API_KEY.", creds.Grant.ExpiresAt.Format(time.DateOnly))
	case creds.Secret != "":
	case f.DryRun:
		creds.Secret = dryRunKey
	case creds.SavedFor != "":
		return nil, exit.MissingKeyf("Your saved login is for %s, not %s. Set CHRONOS_API_KEY to use a key with %s.", creds.SavedFor, creds.BaseURL, creds.BaseURL)
	default:
		return nil, exit.MissingKeyf("You are not logged in. Run chronos auth login, or set CHRONOS_API_KEY.")
	}
	c, err := f.ClientWith(creds)
	if err == nil && creds.Type == config.CredentialOAuthToken {
		f.oauthToken = true
	}
	return c, err
}

func controlCharactersError(source config.Source) error {
	if source == config.SourceEnv {
		return exit.MissingKeyf("The API key in CHRONOS_API_KEY contains control characters. Copy it again from the dashboard.")
	}
	return exit.MissingKeyf("Your saved login contains control characters. Run chronos auth login again.")
}

// ClientWith returns a client for creds. All clients in a run share one
// recorder, so Sent covers every request.
func (f *Factory) ClientWith(creds config.Credentials) (*chronos.Client, error) {
	if err := config.CheckBaseURL(creds.BaseURL); err != nil {
		return nil, err
	}
	opts := append([]chronos.Option{
		chronos.WithBaseURL(creds.BaseURL),
		chronos.WithHTTPClient(&http.Client{Transport: f.transport()}),
		chronos.WithUserAgent(userAgent()),
	}, f.ClientOptions...)
	if f.DryRun {
		// The SDK retries a failed GET, and a captured request is a failure.
		opts = append(opts, chronos.WithMaxRequestRetries(0))
	}
	c, err := chronos.NewClient(creds.Secret, opts...)
	if err != nil {
		return nil, exit.Usagef("%s", exit.Sentence(strings.TrimPrefix(err.Error(), "chronos: ")))
	}
	return c, nil
}

// OAuthClient shares the run's recorder, so Sent sees the OAuth requests too.
func (f *Factory) OAuthClient(baseURL string) (*oauth.Client, error) {
	if err := config.CheckBaseURL(baseURL); err != nil {
		return nil, err
	}
	return &oauth.Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Transport: f.transport(), Timeout: chronos.DefaultRequestTimeout},
		UserAgent:  userAgent(),
	}, nil
}

func (f *Factory) PingClient() *ping.Client {
	return &ping.Client{
		HTTPClient: &http.Client{Transport: f.transport()},
		UserAgent:  userAgent(),
		Timeout:    f.PingTimeout,
	}
}

func (f *Factory) transport() *transport.Recorder {
	if f.recorder == nil {
		f.recorder = &transport.Recorder{}
	}
	f.recorder.DryRun = f.DryRun
	return f.recorder
}

func userAgent() string {
	return "chronos-cli/" + build.Current().Version
}

// Sent reports what this run has sent, for classifying a failure.
func (f *Factory) Sent() exit.Sent {
	if f.recorder == nil {
		return exit.Sent{}
	}
	return exit.Sent{LastMethod: f.recorder.LastMethod(), WriteApplied: f.recorder.WriteApplied(), OAuthToken: f.oauthToken}
}
