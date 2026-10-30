package auth

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/output"
)

const maxKeyLength = 1024

func newLoginCmd(f *app.Factory) *cobra.Command {
	var withKey bool
	spec := cmdutil.CommandSpec{
		Use:   "login",
		Short: "Check an API key and save it for later commands",
		Long: "Check an API key with the API and save it in the keychain, or in the config file on a machine that has no keychain. " +
			"In a terminal, chronos asks for the key without showing it. " +
			"The key is saved for the base URL in use, set by --base-url or CHRONOS_BASE_URL. " +
			"To log in from a script, pipe the key in with --with-key, or skip login and set CHRONOS_API_KEY.",
		Example: "  chronos auth login\n  chronos auth login --with-key < key.txt",
	}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, _ []string) (output.View, error) {
		if withKey && f.IO.InTTY {
			return nil, exit.Usagef("--with-key reads the key from standard input, which is a terminal here, so the key would show as you paste it. Run chronos auth login without --with-key to paste it hidden, or pipe the key in with chronos auth login --with-key < key.txt.")
		}
		if !withKey && (f.JSON || !f.IO.InTTY) {
			return nil, exit.Usagef("chronos auth login asks for the key only in a terminal and never with --json. Pipe the key in with --with-key, or set CHRONOS_API_KEY instead of logging in.")
		}
		baseURL, source := config.ResolveBaseURLSource(f.Getenv, f.BaseURL)
		if err := config.CheckBaseURL(baseURL); err != nil {
			return nil, err
		}
		p := f.Printer()
		if !config.IsDefaultBaseURL(baseURL) {
			p.Note("Logging in to %s, set by %s.", host(baseURL), source)
		}
		key, err := readKey(ctx, f, withKey)
		if err != nil {
			return nil, err
		}
		c, err := f.ClientWith(config.Credentials{APIKey: key, BaseURL: baseURL})
		if err != nil {
			return nil, err
		}
		account, err := c.Account.Get(ctx)
		if err != nil {
			return nil, rejectedKey(err, "The API rejected this key, so it was not saved. Copy the whole key from the dashboard and try again.")
		}
		dir, err := f.ConfigDir()
		if err != nil {
			return nil, err
		}
		saved, err := config.SaveAPIKey(config.SaveOptions{Dir: dir, Keychain: f.Keychain, BaseURL: baseURL, APIKey: key})
		if err != nil {
			return nil, err
		}
		for _, warning := range saved.Warnings {
			p.Note("Warning: %s", warning)
		}
		if config.EnvAPIKey(f.Getenv) != "" {
			p.Note("CHRONOS_API_KEY is set, so commands keep using that key until you unset it.")
		}
		return statusView(saved.Credentials, account), nil
	})
	cmd.Flags().BoolVar(&withKey, "with-key", false, "Read the API key from standard input")
	return cmd
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
	case len(key) > maxKeyLength, strings.ContainsFunc(key, unicode.IsControl):
		return "", exit.Usagef("That is not an API key. Copy the key again from the dashboard.")
	}
	return key, nil
}

func readRawKey(ctx context.Context, f *app.Factory, fromStdin bool) (string, error) {
	if fromStdin {
		b, err := io.ReadAll(io.LimitReader(f.IO.In, maxKeyLength+1))
		return string(b), err
	}
	fmt.Fprint(f.IO.ErrOut, "Paste your API key: ")
	secret, err := f.IO.ReadSecret(ctx)
	fmt.Fprintln(f.IO.ErrOut)
	return secret, err
}
