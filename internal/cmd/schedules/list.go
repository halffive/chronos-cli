package schedules

import (
	"context"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

func newListCmd(f *app.Factory) *cobra.Command {
	var params chronos.ScheduleListParams
	var status string
	var all bool
	spec := cmdutil.CommandSpec{Use: "list", Short: "List schedules, newest first"}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, _ []string) (output.View, error) {
		params.Status = chronos.ScheduleStatus(status)
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		var page *chronos.Page[*chronos.Schedule]
		if all {
			page, err = app.CollectAll(c.Schedules.Iterate(ctx, params))
		} else {
			page, err = c.Schedules.List(ctx, params)
		}
		if err != nil {
			return nil, err
		}
		return output.List(page, "schedules", columns), nil
	})
	flags := cmd.Flags()
	flags.StringVar(&status, "status", "", "Only schedules with this status, such as paused")
	flags.StringVar(&params.Search, "search", "", "Only schedules whose name contains this text, ignoring case")
	flags.IntVar(&params.Page, "page", 0, "Page to show, starting at 1")
	flags.IntVar(&params.Size, "size", 0, "Schedules per page, 1 to 100 (the API's default is 20)")
	flags.BoolVar(&all, "all", false, "List every page")
	cmd.MarkFlagsMutuallyExclusive("page", "all")
	_ = cmd.RegisterFlagCompletionFunc("status", cmdutil.CompleteValues(
		chronos.ScheduleStatusActive, chronos.ScheduleStatusPaused, chronos.ScheduleStatusCompleted, chronos.ScheduleStatusArchived,
	))
	return cmd
}
