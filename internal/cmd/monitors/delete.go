package monitors

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

func newDeleteCmd(f *app.Factory) *cobra.Command {
	var yes bool
	spec := cmdutil.CommandSpec{
		Use:     "delete <id>",
		Short:   "Delete a monitor and its pings permanently; its ping URL stops working",
		Args:    cmdutil.ExactArgs("id"),
		Mutates: true,
	}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, args []string) (output.View, error) {
		id := args[0]
		willPrompt := f.WillPrompt(yes)
		if !willPrompt {
			if err := f.Confirm(ctx, fmt.Sprintf("Delete monitor %s?", id), yes); err != nil {
				return nil, err
			}
		}
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		if willPrompt {
			m, err := c.Monitors.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			if err := f.Confirm(ctx, fmt.Sprintf("Delete monitor %q (%s)?", m.Name, id), yes); err != nil {
				return nil, err
			}
		}
		if err := c.Monitors.Delete(ctx, id); err != nil {
			return nil, err
		}
		return output.Deleted("monitor", id), nil
	})
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Delete without asking")
	return cmd
}
