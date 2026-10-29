package schedules

import (
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
)

func NewCmd(f *app.Factory) *cobra.Command {
	return cmdutil.NewGroup("schedules", "Create, run and manage schedules")
}
