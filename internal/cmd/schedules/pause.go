package schedules

import (
	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
)

func newPauseCmd(f *app.Factory) *cobra.Command {
	return newStatusCmd(f, statusChange{
		Use:         "pause <id>",
		Short:       "Stop a schedule from creating jobs until you resume it",
		Apply:       (*chronos.SchedulesService).Pause,
		AlreadyCode: chronos.ErrorCodeScheduleAlreadyPaused,
		Status:      chronos.ScheduleStatusPaused,
	})
}
