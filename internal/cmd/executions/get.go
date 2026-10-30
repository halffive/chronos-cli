package executions

import (
	"context"

	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

func newGetCmd(f *app.Factory) *cobra.Command {
	spec := cmdutil.CommandSpec{Use: "get <id>", Short: "Show an execution", Args: cmdutil.ExactArgs("id")}
	return cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, args []string) (output.View, error) {
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		e, err := c.Executions.Get(ctx, args[0])
		if err != nil {
			return nil, err
		}
		return executionView(e), nil
	})
}
