package monitors

import (
	"context"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

func newListPingsCmd(f *app.Factory) *cobra.Command {
	var params chronos.MonitorPingListParams
	var all bool
	spec := cmdutil.CommandSpec{
		Use:   "list-pings <id>",
		Short: "List a monitor's pings, newest first; Chronos keeps the latest 1000",
		Args:  cmdutil.ExactArgs("id"),
	}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, args []string) (output.View, error) {
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		var page *chronos.Page[*chronos.MonitorPing]
		if all {
			page, err = app.CollectAll(c.Monitors.IteratePings(ctx, args[0], params))
		} else {
			page, err = c.Monitors.ListPings(ctx, args[0], params)
		}
		if err != nil {
			return nil, err
		}
		return output.List(page, "pings", pingColumns), nil
	})
	flags := cmd.Flags()
	flags.IntVar(&params.Page, "page", 0, "Page to show, starting at 1")
	flags.IntVar(&params.Size, "size", 0, "Pings per page, 1 to 100 (the API's default is 20)")
	flags.BoolVar(&all, "all", false, "List every page")
	cmd.MarkFlagsMutuallyExclusive("page", "all")
	return cmd
}
