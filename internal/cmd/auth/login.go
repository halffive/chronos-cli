package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/oauth"
	"chronos.sh/cli/internal/output"
)

const loginScope = "schedules:write jobs:write executions:read monitors:write account:read"

const maxDeviceNameLength = 64

type loginFlags struct {
	withKey  bool
	pasteKey bool
}

func newLoginCmd(f *app.Factory) *cobra.Command {
	var flags loginFlags
	spec := cmdutil.CommandSpec{
		Use:   "login",
		Short: "Log in with the browser, or save an API key",
		Long: "Without flags, in a terminal, chronos opens the browser, where you log in and pick a workspace. " +
			"A browser login acts as you, with your role in that workspace, until it expires or you run chronos auth logout. " +
			"To save an API key instead, paste it hidden with --paste-key, or pipe it in with --with-key. " +
			"chronos saves the login in the keychain, or in the config file on a machine that has no keychain. " +
			"The login is saved for the base URL in use, set by --base-url or CHRONOS_BASE_URL. " +
			"Scripts, CI and agents should skip login and set CHRONOS_API_KEY.",
		Example: "  chronos auth login\n  chronos auth login --paste-key\n  chronos auth login --with-key < key.txt",
	}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, _ []string) (output.View, error) {
		if err := checkLoginFlags(f, flags); err != nil {
			return nil, err
		}
		baseURL, source := config.ResolveBaseURLSource(f.Getenv, f.BaseURL)
		if err := config.CheckBaseURL(baseURL); err != nil {
			return nil, err
		}
		p := f.Printer()
		if !config.IsDefaultBaseURL(baseURL) {
			p.Note("Logging in to %s, set by %s.", host(baseURL), source)
		}
		dir, err := f.ConfigDir()
		if err != nil {
			return nil, err
		}
		file, err := config.Load(dir)
		if err != nil {
			return nil, err
		}
		previous := savedOAuthToken(f, file)
		var view output.View
		var saved config.SaveResult
		if flags.withKey || flags.pasteKey {
			view, saved, err = loginWithKey(ctx, f, loginTarget{dir: dir, baseURL: baseURL}, flags.withKey)
		} else {
			view, saved, err = loginWithBrowser(ctx, f, loginTarget{dir: dir, baseURL: baseURL})
		}
		if err != nil {
			return nil, err
		}
		if previous.Secret != "" {
			revokeBestEffort(ctx, f, previous)
		}
		for _, warning := range saved.Warnings {
			p.Note("Warning: %s", warning)
		}
		if config.EnvAPIKey(f.Getenv) != "" {
			p.Note("CHRONOS_API_KEY is set, so commands keep using that key until you unset it.")
		}
		return view, nil
	})
	cmd.Flags().BoolVar(&flags.withKey, "with-key", false, "Read the API key from standard input")
	cmd.Flags().BoolVar(&flags.pasteKey, "paste-key", false, "Paste an API key in the terminal instead of logging in with the browser")
	cmd.MarkFlagsMutuallyExclusive("with-key", "paste-key")
	return cmd
}

func checkLoginFlags(f *app.Factory, flags loginFlags) error {
	interactive := f.IO.InTTY && !f.JSON
	switch {
	case flags.withKey && f.IO.InTTY:
		return exit.Usagef("--with-key reads the key from standard input, which is a terminal here, so the key would show as you paste it. Run chronos auth login --paste-key to paste it hidden, or pipe the key in with chronos auth login --with-key < key.txt.")
	case flags.pasteKey && !interactive:
		return exit.Usagef("--paste-key asks for the key only in a terminal and never with --json. Pipe the key in with --with-key, or set CHRONOS_API_KEY instead of logging in.")
	case !flags.withKey && !flags.pasteKey && !interactive:
		return exit.Usagef("Browser login runs only from a terminal and never with --json. Pipe an API key in with --with-key, or set CHRONOS_API_KEY instead of logging in.")
	}
	return nil
}

type loginTarget struct {
	dir     string
	baseURL string
}

func savedOAuthToken(f *app.Factory, file *config.File) config.Credentials {
	saved, ok := file.Saved()
	if !ok || saved.Type != config.CredentialOAuthToken {
		return config.Credentials{}
	}
	saved, err := config.LoadSecret(f.Keychain, saved)
	if err != nil {
		return config.Credentials{}
	}
	return saved
}

func revokeBestEffort(ctx context.Context, f *app.Factory, creds config.Credentials) {
	client, err := f.OAuthClient(creds.BaseURL)
	if err != nil {
		return
	}
	client.RevokeBestEffort(ctx, creds.Secret)
}

func loginWithKey(ctx context.Context, f *app.Factory, target loginTarget, fromStdin bool) (output.View, config.SaveResult, error) {
	key, err := readKey(ctx, f, fromStdin)
	if err != nil {
		return nil, config.SaveResult{}, err
	}
	c, err := f.ClientWith(config.Credentials{Secret: key, BaseURL: target.baseURL})
	if err != nil {
		return nil, config.SaveResult{}, err
	}
	account, err := c.Account.Get(ctx)
	if err != nil {
		return nil, config.SaveResult{}, rejectedKey(err, "The API rejected this key, so it was not saved. Copy the whole key from the dashboard and try again.")
	}
	saved, err := config.SaveCredential(config.SaveOptions{Dir: target.dir, Keychain: f.Keychain, BaseURL: target.baseURL, Type: config.CredentialAPIKey, Secret: key})
	if err != nil {
		return nil, config.SaveResult{}, err
	}
	message := fmt.Sprintf("Logged in to %s with API key %s.", output.CleanLine(account.Name), keyHint(saved.Credentials.Secret))
	return loggedInView(statusOf(saved.Credentials, account), message), saved, nil
}

func loginWithBrowser(ctx context.Context, f *app.Factory, target loginTarget) (output.View, config.SaveResult, error) {
	client, err := f.OAuthClient(target.baseURL)
	if err != nil {
		return nil, config.SaveResult{}, err
	}
	timeout := f.LoginTimeout
	if timeout == 0 {
		timeout = app.DefaultLoginTimeout
	}
	p := f.Printer()
	token, err := client.Login(ctx, oauth.LoginOptions{
		Scope:      loginScope,
		DeviceName: deviceName(),
		Timeout:    timeout,
		OpenBrowser: func(authorizeURL string) {
			p.Note("Opening the browser to log in. If it does not open, visit this URL:\n%s", authorizeURL)
			if err := openBrowser(f, authorizeURL); err != nil {
				p.Note("Could not open the browser (%v). Visit the URL above to log in.", err)
			}
			p.Note("Waiting for you to approve the login in the browser. Press Ctrl-C to cancel.")
		},
		Ignored: func() {
			p.Note("Ignored a request that was not part of this login. Still waiting for the browser.")
		},
	})
	if err != nil {
		return nil, config.SaveResult{}, err
	}
	grant := config.Grant{User: token.User, Account: token.Account, Scopes: token.Scopes}
	if token.ExpiresIn > 0 {
		grant.ExpiresAt = f.Now().Add(token.ExpiresIn).UTC().Truncate(time.Second)
	}
	saved, err := config.SaveCredential(config.SaveOptions{
		Dir:      target.dir,
		Keychain: f.Keychain,
		BaseURL:  target.baseURL,
		Type:     config.CredentialOAuthToken,
		Secret:   token.AccessToken,
		Grant:    grant,
	})
	if err != nil {
		client.RevokeBestEffort(ctx, token.AccessToken)
		return nil, config.SaveResult{}, err
	}
	message := fmt.Sprintf("Logged in as %s to %s.", output.CleanLine(saved.Credentials.Grant.User.Email), output.CleanLine(saved.Credentials.Grant.Account.Name))
	return loggedInView(nil, message), saved, nil
}

func loggedInView(status any, message string) output.View {
	return output.ViewOf(status, func(p *output.Printer) error {
		_, err := fmt.Fprintln(p.Out, message)
		return err
	})
}

func openBrowser(f *app.Factory, url string) error {
	if f.OpenBrowser == nil {
		return errors.New("no browser is available")
	}
	return f.OpenBrowser(url)
}

// deviceName keeps only printable ASCII, since the API rejects any other
// device_name.
func deviceName() string {
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}
	name := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e {
			return -1
		}
		return r
	}, hostname)
	return strings.TrimSpace(name[:min(len(name), maxDeviceNameLength)])
}

func host(baseURL string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return baseURL
	}
	return u.Host
}

func readKey(ctx context.Context, f *app.Factory, fromStdin bool) (string, error) {
	raw, err := readRawKey(ctx, f, fromStdin)
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(raw)
	switch {
	case key == "":
		return "", exit.Usagef("No API key was given. Copy a key from the dashboard and try again.")
	case len(key) > config.MaxSecretLength, strings.ContainsFunc(key, unicode.IsControl):
		return "", exit.Usagef("That is not an API key. Copy the key again from the dashboard.")
	}
	return key, nil
}

func readRawKey(ctx context.Context, f *app.Factory, fromStdin bool) (string, error) {
	if fromStdin {
		return app.ReadWithContext(ctx, func() (string, error) {
			b, err := io.ReadAll(io.LimitReader(f.IO.In, config.MaxSecretLength+1))
			return string(b), err
		})
	}
	fmt.Fprint(f.IO.ErrOut, "Paste your API key: ")
	secret, err := f.IO.ReadSecret(ctx)
	fmt.Fprintln(f.IO.ErrOut)
	return secret, err
}
