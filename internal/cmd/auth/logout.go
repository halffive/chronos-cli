package auth

import (
	"context"
	"errors"
	"fmt"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/output"
)

type logoutJSON struct {
	LoggedOut bool `json:"logged_out"`
}

func newLogoutCmd(f *app.Factory) *cobra.Command {
	spec := cmdutil.CommandSpec{
		Use:   "logout",
		Short: "Log out and remove the saved login",
		Long: "Revoke a browser login on the server, then remove the saved login from the keychain and the config file. " +
			"A saved API key is removed but stays valid until you revoke it in the dashboard.",
	}
	return cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, _ []string) (output.View, error) {
		dir, err := f.ConfigDir()
		if err != nil {
			return nil, err
		}
		file, err := config.Load(dir)
		if err != nil {
			return nil, err
		}
		reason, revokeErr := revokeSaved(ctx, f, file)
		if errors.Is(revokeErr, context.Canceled) {
			return nil, revokeErr
		}
		if e, ok := errors.AsType[*exit.Error](revokeErr); ok && e.Exit == exit.Retryable {
			return nil, stillSaved(e)
		}
		result, err := config.RemoveProfile(dir, f.Keychain)
		if err != nil {
			return nil, err
		}
		removed := result.Removed
		p := f.Printer()
		if !removed {
			p.Note("There is no saved login to remove.")
		}
		for _, warning := range result.Warnings {
			p.Note("Warning: %s", warning)
		}
		if config.EnvAPIKey(f.Getenv) != "" {
			p.Note("CHRONOS_API_KEY is set, so commands still use that key.")
		}
		view := output.ViewOf(logoutJSON{LoggedOut: removed}, func(p *output.Printer) error {
			if !removed {
				return nil
			}
			_, err := fmt.Fprintln(p.Out, "Logged out.")
			return err
		})
		if revokeErr != nil {
			return view, notRevoked(revokeErr, reason)
		}
		return view, nil
	})
}

// revokeSaved revokes a saved OAuth token at the base URL it was saved for.
// On failure it also returns a short reason for notRevoked.
func revokeSaved(ctx context.Context, f *app.Factory, file *config.File) (reason string, err error) {
	saved, ok := file.Saved()
	if !ok || saved.Type != config.CredentialOAuthToken {
		return "", nil
	}
	saved, err = config.LoadSecret(f.Keychain, saved)
	if err != nil {
		return fmt.Sprintf("could not read it from the keychain: %v", errors.Unwrap(err)), err
	}
	client, err := f.OAuthClient(saved.BaseURL)
	if err != nil {
		return "the saved base URL is not usable", err
	}
	if err := client.Revoke(ctx, saved.Secret); err != nil {
		return revokeReason(err), err
	}
	return "", nil
}

func revokeReason(err error) string {
	apiErr, ok := errors.AsType[*chronos.APIError](err)
	switch {
	case !ok:
		return "no reason given"
	case apiErr.Code != "":
		return string(apiErr.Code)
	}
	return fmt.Sprintf("the API answered %d", apiErr.StatusCode)
}

func stillSaved(e *exit.Error) error {
	return &exit.Error{
		Exit:      e.Exit,
		Code:      e.Code,
		Retryable: e.Retryable,
		Cause:     e.Cause,
		Message:   fmt.Sprintf("Could not revoke the login on the server, so it is still saved. %s Run chronos auth logout again.", exit.Sentence(e.Message)),
	}
}

func notRevoked(err error, reason string) error {
	message := fmt.Sprintf("Removed the saved login, but could not revoke it on the server (%s). Revoke it on the CLI logins page in the dashboard.", reason)
	e, ok := errors.AsType[*exit.Error](err)
	if !ok {
		return exit.Failuref("%s", message)
	}
	return &exit.Error{Exit: e.Exit, Code: e.Code, Message: message, Cause: e.Cause}
}
