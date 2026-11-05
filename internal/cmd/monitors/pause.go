package monitors

import (
	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
)

func newPauseCmd(f *app.Factory) *cobra.Command {
	return newStatusCmd(f, statusChange{
		Use:         "pause <id>",
		Short:       "Stop a monitor from alerting until you resume it",
		Long:        "Stop a monitor from alerting until you resume it. Pings that arrive while it is paused still show as its last ping, but list-pings does not include them.",
		Apply:       (*chronos.MonitorsService).Pause,
		AlreadyCode: chronos.ErrorCodeMonitorAlreadyPaused,
		Reached:     func(s chronos.MonitorStatus) bool { return s == chronos.MonitorStatusPaused },
		State:       "already paused",
	})
}
