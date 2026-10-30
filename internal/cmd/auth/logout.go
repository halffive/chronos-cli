package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/output"
)

type logoutJSON struct {
	LoggedOut bool `json:"logged_out"`
}

func newLogoutCmd(f *app.Factory) *cobra.Command {
	spec := cmdutil.CommandSpec{
		Use:   "logout",
		Short: "Remove the saved API key",
		Long:  "Remove the saved API key from the keychain and the config file. The key stays valid until you revoke it in the dashboard.",
	}
	return cmdutil.NewCommand(f, spec, func(context.Context, *cobra.Command, []string) (output.View, error) {
		dir, err := f.ConfigDir()
		if err != nil {
			return nil, err
		}
		result, err := config.RemoveProfile(dir, f.Keychain)
		if err != nil {
			return nil, err
		}
		removed := result.Removed
		p := f.Printer()
		if !removed {
			p.Note("There is no saved API key to remove.")
		}
		for _, warning := range result.Warnings {
			p.Note("Warning: %s", warning)
		}
		if config.EnvAPIKey(f.Getenv) != "" {
			p.Note("CHRONOS_API_KEY is set, so commands still use that key.")
		}
		return output.ViewOf(logoutJSON{LoggedOut: removed}, func(p *output.Printer) error {
			if !removed {
				return nil
			}
			_, err := fmt.Fprintln(p.Out, "Logged out.")
			return err
		}), nil
	})
}
