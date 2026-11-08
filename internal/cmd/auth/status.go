package auth

import (
	"context"

	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/output"
)

func newStatusCmd(f *app.Factory) *cobra.Command {
	spec := cmdutil.CommandSpec{
		Use:   "status",
		Short: "Show which login or API key is in use and the workspace it belongs to",
		Long: "Show which login or API key is in use, where it came from, and the workspace it belongs to. " +
			"For a browser login, also show who it acts as, its scopes and when it expires. " +
			"CHRONOS_API_KEY overrides a saved login.",
	}
	return cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, _ []string) (output.View, error) {
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		creds, err := f.Credentials()
		if err != nil {
			return nil, err
		}
		account, err := c.Account.Get(ctx)
		switch {
		case err == nil:
			return statusView(creds, account), nil
		case creds.Type == config.CredentialOAuthToken:
			return nil, err
		}
		return nil, rejectedKey(err, rejectedMessage(creds))
	})
}
