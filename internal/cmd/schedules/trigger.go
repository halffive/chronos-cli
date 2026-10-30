package schedules

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/input"
	"chronos.sh/cli/internal/output"
)

func newTriggerCmd(f *app.Factory) *cobra.Command {
	var wait bool
	var waitTimeout string
	spec := cmdutil.CommandSpec{
		Use:   "trigger <id>",
		Short: "Run a schedule now by creating a job",
		Long: "Run a schedule now by creating a job. The job counts toward --runs.\n\n" +
			"With --wait, the command exits 8 when the job failed, was cancelled or was skipped, " +
			"and 9 when --wait-timeout runs out before the job finishes.",
		Args:    cmdutil.ExactArgs("id"),
		Mutates: true,
	}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, cmd *cobra.Command, args []string) (output.View, error) {
		var timeout time.Duration
		if !wait && cmd.Flags().Changed("wait-timeout") {
			return nil, exit.Usagef("--wait-timeout only applies with --wait.")
		}
		if wait {
			n, err := input.ParseSeconds(waitTimeout)
			if err != nil {
				return nil, err
			}
			timeout = time.Duration(n) * time.Second
			if err := app.CheckWaitTimeout(timeout); err != nil {
				return nil, err
			}
		}
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		job, err := c.Schedules.Trigger(ctx, args[0])
		if err != nil {
			return nil, err
		}
		if !wait {
			return jobView(job), nil
		}
		job, err = f.WaitForJob(ctx, c, app.WaitOptions{JobID: job.ID, Timeout: timeout, Job: job})
		if job == nil {
			return nil, err
		}
		return jobView(job), err
	})
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for the job to finish")
	cmd.Flags().StringVar(&waitTimeout, "wait-timeout", seconds(int(app.DefaultWaitTimeout.Seconds())), "How long to wait for the job to finish, such as 90s or 5m")
	return cmd
}
