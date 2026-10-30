package auth

import (
	"context"

	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

func newStatusCmd(f *app.Factory) *cobra.Command {
	spec := cmdutil.CommandSpec{
		Use:   "status",
		Short: "Show which API key is in use and the workspace it belongs to",
		Long:  "Show which API key is in use, where it came from, and the workspace it belongs to. CHRONOS_API_KEY overrides a saved key.",
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
		if err != nil {
			return nil, rejectedKey(err, rejectedMessage(creds))
		}
		return statusView(creds, account), nil
	})
}
