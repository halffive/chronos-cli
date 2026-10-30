package jobs

import (
	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
)

func newSkipCmd(f *app.Factory) *cobra.Command {
	return newActionCmd(f, cmdutil.CommandSpec{
		Use:   "skip <id>",
		Short: "Skip a job a schedule created, before it runs",
		Long:  "Skip a job a schedule created, before it runs. Only pending or queued jobs can be skipped.",
	}, (*chronos.JobsService).Skip)
}
