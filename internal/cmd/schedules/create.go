package schedules

import (
	"context"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

func newCreateCmd(f *app.Factory) *cobra.Command {
	var flags bodyFlags
	spec := cmdutil.CommandSpec{
		Use:   "create",
		Short: "Create a schedule",
		Long: "Create a schedule that runs on a cron expression or an interval. A worker pulls its jobs by --handler unless --push-url pushes them to your endpoint.\n\n" +
			"--file takes a full API request body, and flags override its fields.",
		Example: `  chronos schedules create --name "Nightly report" --cron "0 3 * * *" --handler report
  chronos schedules create --name Sync --interval 5m --push-url https://example.com/hooks/sync
  chronos schedules create --file schedule.json`,
		Mutates: true,
	}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, cmd *cobra.Command, _ []string) (output.View, error) {
		var params chronos.ScheduleCreateParams
		b, err := flags.read(cmd.Flags(), f.IO.In, &params)
		if err != nil {
			return nil, err
		}
		b.applyCreate(&params)
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		s, err := c.Schedules.Create(ctx, params)
		if err != nil {
			return nil, err
		}
		return scheduleView(s), nil
	})
	flags.register(cmd)
	return cmd
}
