package jobs

import (
	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
)

func newRetryCmd(f *app.Factory) *cobra.Command {
	return newActionCmd(f, cmdutil.CommandSpec{
		Use:   "retry <id>",
		Short: "Run a failed job again now",
	}, (*chronos.JobsService).Retry)
}
