package jobs

import (
	"context"
	"time"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/input"
	"chronos.sh/cli/internal/output"
)

func newListCmd(f *app.Factory) *cobra.Command {
	var params chronos.JobListParams
	var origin, status, scheduledAfter, scheduledBefore string
	var all bool
	spec := cmdutil.CommandSpec{
		Use:   "list",
		Short: "List jobs, newest first",
		Example: "  chronos jobs list --status failed --scheduled-after 24h\n" +
			"  chronos jobs list --schedule-id <id> --all --json",
	}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, _ []string) (output.View, error) {
		var err error
		if params.ScheduledAfter, err = parseTimeFilter(scheduledAfter, f.Now()); err != nil {
			return nil, err
		}
		if params.ScheduledBefore, err = parseTimeFilter(scheduledBefore, f.Now()); err != nil {
			return nil, err
		}
		params.Origin = chronos.JobOrigin(origin)
		params.Status = chronos.JobStatus(status)
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		var page *chronos.Page[*chronos.Job]
		if all {
			page, err = app.CollectAll(c.Jobs.Iterate(ctx, params))
		} else {
			page, err = c.Jobs.List(ctx, params)
		}
		if err != nil {
			return nil, err
		}
		return output.List(page, "jobs", columns), nil
	})

	flags := cmd.Flags()
	flags.StringVar(&params.ScheduleID, "schedule-id", "", "Only jobs created by this schedule")
	flags.StringVar(&origin, "origin", "", "Only jobs created through the API (api) or by a schedule (schedule)")
	flags.StringVar(&params.Handler, "handler", "", "Only jobs for this handler")
	flags.StringVar(&params.Search, "search", "", "Only jobs whose name contains this text, ignoring case")
	flags.StringVar(&status, "status", "", "Only jobs with this status, such as failed")
	flags.StringVar(&scheduledAfter, "scheduled-after", "", "Only jobs scheduled at or after this time: RFC 3339, or a duration such as 24h for that long ago")
	flags.StringVar(&scheduledBefore, "scheduled-before", "", "Only jobs scheduled at or before this time: RFC 3339, or a duration such as 24h for that long ago")
	flags.IntVar(&params.Page, "page", 0, "Page to show, starting at 1")
	flags.IntVar(&params.Size, "size", 0, "Jobs per page, 1 to 100 (the API's default is 20)")
	flags.BoolVar(&all, "all", false, "List every page")
	cmd.MarkFlagsMutuallyExclusive("page", "all")
	_ = cmd.RegisterFlagCompletionFunc("status", cmdutil.CompleteValues(statuses...))
	_ = cmd.RegisterFlagCompletionFunc("origin", cmdutil.CompleteValues(chronos.JobOriginAPI, chronos.JobOriginSchedule))
	return cmd
}

func parseTimeFilter(s string, now time.Time) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return input.ParseTimeFilter(s, now)
}
