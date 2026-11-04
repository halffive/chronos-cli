package monitors

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
		Short: "Create a monitor",
		Long: "Create a monitor for a job that runs on a cron expression or an interval. Your job calls the ping URL in the output, and Chronos alerts when a ping reports failure or none arrives in time. A new monitor sends no alert until its first success or fail ping.\n\n" +
			"--file takes a full API request body, and flags override its fields.",
		Example: `  chronos monitors create --name "Nightly backup" --cron "0 3 * * *" --grace 10m
  chronos monitors create --name Sync --interval 1h
  chronos monitors create --file monitor.json`,
		Mutates: true,
	}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, cmd *cobra.Command, _ []string) (output.View, error) {
		var params chronos.MonitorCreateParams
		b, err := flags.read(cmd.Flags(), f.IO.In, &params)
		if err != nil {
			return nil, err
		}
		b.applyCreate(&params)
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		m, err := c.Monitors.Create(ctx, params)
		if err != nil {
			return nil, err
		}
		return monitorView(m, fullPingURL), nil
	})
	flags.register(cmd)
	return cmd
}
