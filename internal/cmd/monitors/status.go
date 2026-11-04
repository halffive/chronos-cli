package monitors

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
	Long        string
	Apply       func(s *chronos.MonitorsService, ctx context.Context, id string) (*chronos.Monitor, error)
	AlreadyCode chronos.ErrorCode
	Reached     func(chronos.MonitorStatus) bool
	State       string
}

func newStatusCmd(f *app.Factory, change statusChange) *cobra.Command {
	spec := cmdutil.CommandSpec{Use: change.Use, Short: change.Short, Long: change.Long, Args: cmdutil.ExactArgs("id"), Mutates: true}
	return cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, args []string) (output.View, error) {
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		m, err := change.Apply(c.Monitors, ctx, args[0])
		if err == nil {
			return monitorView(m, maskedPingURL), nil
		}
		if !isConflict(err, change.AlreadyCode) {
			return nil, err
		}
		conflict := err
		if m, err = c.Monitors.Get(ctx, args[0]); err != nil {
			return nil, err
		}
		if !change.Reached(m.Status) {
			return nil, conflict
		}
		f.Printer().Note("Monitor %s is %s, so nothing changed.", args[0], change.State)
		return monitorView(m, maskedPingURL), nil
	})
}

func isConflict(err error, code chronos.ErrorCode) bool {
	apiErr, ok := errors.AsType[*chronos.APIError](err)
	return ok && apiErr.StatusCode == http.StatusConflict && apiErr.Code == code
}
