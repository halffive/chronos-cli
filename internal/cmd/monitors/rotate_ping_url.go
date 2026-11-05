package monitors

import (
	"context"

	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

func newRotatePingURLCmd(f *app.Factory) *cobra.Command {
	spec := cmdutil.CommandSpec{
		Use:   "rotate-ping-url <id>",
		Short: "Replace a monitor's ping URL; the old one stops accepting pings at once",
		Long: "Replace a monitor's ping URL. The old URL stops accepting pings at once, so update your job with the new ping URL in the output.\n\n" +
			"If the command exits 7 or 130, the rotation may have happened. Run chronos monitors get <id> to read the current URL.",
		Args:    cmdutil.ExactArgs("id"),
		Mutates: true,
	}
	return cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, args []string) (output.View, error) {
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		m, err := c.Monitors.RotatePingURL(ctx, args[0])
		if err != nil {
			return nil, err
		}
		return monitorView(m, fullPingURL), nil
	})
}
