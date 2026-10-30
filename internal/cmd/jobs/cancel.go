package jobs

import (
	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
)

func newCancelCmd(f *app.Factory) *cobra.Command {
	return newActionCmd(f, cmdutil.CommandSpec{
		Use:   "cancel <id>",
		Short: "Cancel a job that is waiting to run",
	}, (*chronos.JobsService).Cancel)
}
