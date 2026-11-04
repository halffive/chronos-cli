package monitors

import (
	"context"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

func newListCmd(f *app.Factory) *cobra.Command {
	var params chronos.MonitorListParams
	var status string
	var all bool
	spec := cmdutil.CommandSpec{Use: "list", Short: "List monitors, newest first"}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, _ []string) (output.View, error) {
		params.Status = chronos.MonitorStatus(status)
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		var page *chronos.Page[*chronos.Monitor]
		if all {
			page, err = app.CollectAll(c.Monitors.Iterate(ctx, params))
		} else {
			page, err = c.Monitors.List(ctx, params)
		}
		if err != nil {
			return nil, err
		}
		return output.List(page, "monitors", columns), nil
	})
	flags := cmd.Flags()
	flags.StringVar(&status, "status", "", "Only monitors with this status, such as down")
	flags.StringVar(&params.Search, "search", "", "Only monitors whose name contains this text, ignoring case")
	flags.IntVar(&params.Page, "page", 0, "Page to show, starting at 1")
	flags.IntVar(&params.Size, "size", 0, "Monitors per page, 1 to 100 (the API's default is 20)")
	flags.BoolVar(&all, "all", false, "List every page")
	cmd.MarkFlagsMutuallyExclusive("page", "all")
	_ = cmd.RegisterFlagCompletionFunc("status", cmdutil.CompleteValues(
		chronos.MonitorStatusNew, chronos.MonitorStatusUp, chronos.MonitorStatusLate, chronos.MonitorStatusDown, chronos.MonitorStatusPaused,
	))
	return cmd
}
