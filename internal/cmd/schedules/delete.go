package schedules

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
		Short:   "Delete a schedule permanently; its jobs remain",
		Args:    cmdutil.ExactArgs("id"),
		Mutates: true,
	}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, args []string) (output.View, error) {
		id := args[0]
		willPrompt := f.WillPrompt(yes)
		if !willPrompt {
			if err := f.Confirm(ctx, fmt.Sprintf("Delete schedule %s?", id), yes); err != nil {
				return nil, err
			}
		}
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		if willPrompt {
			s, err := c.Schedules.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			if err := f.Confirm(ctx, fmt.Sprintf("Delete schedule %q (%s)?", s.Name, id), yes); err != nil {
				return nil, err
			}
		}
		if err := c.Schedules.Delete(ctx, id); err != nil {
			return nil, err
		}
		return output.Deleted("schedule", id), nil
	})
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Delete without asking")
	return cmd
}
