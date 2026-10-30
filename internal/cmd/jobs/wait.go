package jobs

import (
	"context"

	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

func newWaitCmd(f *app.Factory) *cobra.Command {
	var waitTimeout string
	spec := cmdutil.CommandSpec{
		Use:   "wait <id>",
		Short: "Wait for a job to finish",
		Long: "Wait for a job to finish and print it. Exits 8 when the job failed, was cancelled or was skipped, " +
			"and 9 when --wait-timeout runs out before the job finishes.",
		Args: cmdutil.ExactArgs("id"),
	}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, args []string) (output.View, error) {
		timeout, err := parseWaitTimeout(waitTimeout)
		if err != nil {
			return nil, err
		}
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		return waitView(f.WaitForJob(ctx, c, app.WaitOptions{JobID: args[0], Timeout: timeout}))
	})
	addWaitTimeoutFlag(cmd, &waitTimeout)
	return cmd
}
