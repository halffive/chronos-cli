package schedules

import (
	"context"

	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmd/jobs"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

func newTriggerCmd(f *app.Factory) *cobra.Command {
	var wait cmdutil.WaitFlags
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
		timeout, err := wait.Timeout()
		if err != nil {
			return nil, err
		}
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		job, err := c.Schedules.Trigger(ctx, args[0])
		if err != nil {
			return nil, err
		}
		if !wait.Wait {
			return jobs.View(job), nil
		}
		return jobs.WaitView(f.WaitForJob(ctx, c, app.WaitOptions{JobID: job.ID, Timeout: timeout, Job: job}))
	})
	wait.Register(cmd)
	return cmd
}
