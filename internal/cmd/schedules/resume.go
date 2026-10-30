package schedules

import (
	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
)

func newResumeCmd(f *app.Factory) *cobra.Command {
	return newStatusCmd(f, statusChange{
		Use:         "resume <id>",
		Short:       "Restart a paused schedule from its next run, skipping runs it missed",
		Apply:       (*chronos.SchedulesService).Resume,
		AlreadyCode: chronos.ErrorCodeScheduleAlreadyActive,
		Status:      chronos.ScheduleStatusActive,
	})
}
