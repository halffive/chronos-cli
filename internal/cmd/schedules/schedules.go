package schedules

import (
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
)

func NewCmd(f *app.Factory) *cobra.Command {
	cmd := cmdutil.NewGroup("schedules", "Create, run and manage schedules")
	cmd.AddCommand(
		newListCmd(f),
		newGetCmd(f),
		newCreateCmd(f),
		newUpdateCmd(f),
		newPauseCmd(f),
		newResumeCmd(f),
		newTriggerCmd(f),
		newArchiveCmd(f),
		newDeleteCmd(f),
	)
	return cmd
}
