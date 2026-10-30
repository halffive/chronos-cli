package schedules

import (
	"context"
	"errors"
	"net/http"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

type statusChange struct {
	Use         string
	Short       string
	Apply       func(s *chronos.SchedulesService, ctx context.Context, id string) (*chronos.Schedule, error)
	AlreadyCode chronos.ErrorCode
	Status      chronos.ScheduleStatus
}

func newStatusCmd(f *app.Factory, change statusChange) *cobra.Command {
	spec := cmdutil.CommandSpec{Use: change.Use, Short: change.Short, Args: cmdutil.ExactArgs("id"), Mutates: true}
	return cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, args []string) (output.View, error) {
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		s, err := change.Apply(c.Schedules, ctx, args[0])
		if err == nil {
			return scheduleView(s), nil
		}
		if !isConflict(err, change.AlreadyCode) {
			return nil, err
		}
		conflict := err
		if s, err = c.Schedules.Get(ctx, args[0]); err != nil {
			return nil, err
		}
		if s.Status != change.Status {
			return nil, conflict
		}
		f.Printer().Note("Schedule %s is already %s, so nothing changed.", args[0], change.Status)
		return scheduleView(s), nil
	})
}

func isConflict(err error, code chronos.ErrorCode) bool {
	apiErr, ok := errors.AsType[*chronos.APIError](err)
	return ok && apiErr.StatusCode == http.StatusConflict && apiErr.Code == code
}
