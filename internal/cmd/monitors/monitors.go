package monitors

import (
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
)

func NewCmd(f *app.Factory) *cobra.Command {
	cmd := cmdutil.NewGroup("monitors", "Monitor cron jobs and scripts that run outside Chronos")
	cmd.AddCommand(
		newListCmd(f),
		newGetCmd(f),
		newCreateCmd(f),
		newUpdateCmd(f),
		newPauseCmd(f),
		newResumeCmd(f),
		newRotatePingURLCmd(f),
		newListPingsCmd(f),
		newExecCmd(f),
		newDeleteCmd(f),
	)
	return cmd
}
