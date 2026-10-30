package schedules

import (
	"context"

	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

func newArchiveCmd(f *app.Factory) *cobra.Command {
	spec := cmdutil.CommandSpec{
		Use:     "archive <id>",
		Short:   "Stop a schedule for good and cancel its jobs waiting to run",
		Args:    cmdutil.ExactArgs("id"),
		Mutates: true,
	}
	return cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, args []string) (output.View, error) {
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		s, err := c.Schedules.Archive(ctx, args[0])
		if err != nil {
			return nil, err
		}
		return scheduleView(s), nil
	})
}
