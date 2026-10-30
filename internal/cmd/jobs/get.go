package jobs

import (
	"context"

	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

func newGetCmd(f *app.Factory) *cobra.Command {
	spec := cmdutil.CommandSpec{Use: "get <id>", Short: "Show a job", Args: cmdutil.ExactArgs("id")}
	return cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, args []string) (output.View, error) {
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		j, err := c.Jobs.Get(ctx, args[0])
		if err != nil {
			return nil, err
		}
		return jobView(j), nil
	})
}
