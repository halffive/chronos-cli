package executions

import (
	"context"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

func newListCmd(f *app.Factory) *cobra.Command {
	var params chronos.ExecutionListParams
	var scheduleID, status string
	var all bool
	spec := cmdutil.CommandSpec{
		Use:   "list",
		Short: "List executions, newest first",
		Example: "  chronos executions list --status failed --created-after 24h\n" +
			"  chronos executions list --job-id <id> --json\n" +
			"  chronos executions list --schedule-id null --status timeout",
	}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, _ []string) (output.View, error) {
		params.ScheduleID = scheduleFilter(scheduleID)
		params.Status = chronos.ExecutionStatus(status)
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		var page *chronos.Page[*chronos.Execution]
		if all {
			page, err = app.CollectAll(c.Executions.Iterate(ctx, params))
		} else {
			page, err = c.Executions.List(ctx, params)
		}
		if err != nil {
			return nil, err
		}
		return output.List(page, "executions", columns), nil
	})

	flags := cmd.Flags()
	flags.StringVar(&params.JobID, "job-id", "", "Only executions of this job")
	flags.StringVar(&scheduleID, "schedule-id", "", "Only executions of jobs created by this schedule, or null for one-off jobs")
	flags.StringVar(&params.Handler, "handler", "", "Only executions of jobs for this handler")
	flags.StringVar(&status, "status", "", "Only executions with this status, such as failed or timeout")
	flags.Var(cmdutil.NewTimeFilter(&params.CreatedAfter, f.Now), "created-after", "Only executions created at or after this time: RFC 3339, or a duration such as 24h for that long ago")
	flags.Var(cmdutil.NewTimeFilter(&params.CreatedBefore, f.Now), "created-before", "Only executions created at or before this time: RFC 3339, or a duration such as 24h for that long ago")
	flags.IntVar(&params.Page, "page", 0, "Page to show, starting at 1")
	flags.IntVar(&params.Size, "size", 0, "Executions per page, 1 to 100 (the API's default is 20)")
	flags.BoolVar(&all, "all", false, "List every page")
	cmd.MarkFlagsMutuallyExclusive("page", "all")
	_ = cmd.RegisterFlagCompletionFunc("status", cmdutil.CompleteValues(statuses...))
	return cmd
}

func scheduleFilter(id string) chronos.Nullable[string] {
	switch id {
	case "":
		return chronos.Nullable[string]{}
	case "null":
		return chronos.Null[string]()
	}
	return chronos.NewNullable(id)
}
