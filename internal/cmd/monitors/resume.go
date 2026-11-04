package monitors

import (
	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
)

func newResumeCmd(f *app.Factory) *cobra.Command {
	return newStatusCmd(f, statusChange{
		Use:         "resume <id>",
		Short:       "Resume a paused monitor; alerting restarts with its next success or fail ping",
		Apply:       (*chronos.MonitorsService).Resume,
		AlreadyCode: chronos.ErrorCodeMonitorNotPaused,
		Reached:     func(s chronos.MonitorStatus) bool { return s != chronos.MonitorStatusPaused },
		State:       "not paused",
	})
}
