package jobs

import (
	"context"
	"io"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/input"
	"chronos.sh/cli/internal/output"
)

type createFlags struct {
	name           string
	handler        string
	delay          string
	scheduledFor   string
	timeout        string
	maxRetries     int
	payload        string
	idempotencyKey string
	push           cmdutil.PushFlags
	file           string
	wait           cmdutil.WaitFlags
}

func newCreateCmd(f *app.Factory) *cobra.Command {
	var o createFlags
	spec := cmdutil.CommandSpec{
		Use:   "create",
		Short: "Create a one-off job",
		Long: "Create a one-off job. It runs now unless --delay or --scheduled-for sets a later time.\n\n" +
			"--file takes a full API request body, and flags override its fields. " +
			"With --wait, the command exits 8 when the job failed, was cancelled or was skipped, " +
			"and 9 when --wait-timeout runs out before the job finishes.",
		Example: "  chronos jobs create --name 'Send digest' --handler send-digest --delay 10m\n" +
			"  chronos jobs create --name Report --push-url https://example.com/hooks/report --payload @body.json --wait\n" +
			"  chronos jobs create --file job.json --idempotency-key digest-2026-11-01",
		Mutates: true,
	}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, cmd *cobra.Command, _ []string) (output.View, error) {
		timeout, err := o.wait.Timeout()
		if err != nil {
			return nil, err
		}
		params, err := o.params(cmd.Flags(), f.IO.In)
		if err != nil {
			return nil, err
		}
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		j, err := c.Jobs.Create(ctx, params)
		if err != nil {
			return nil, err
		}
		if !o.wait.Wait {
			return View(j), nil
		}
		return WaitView(f.WaitForJob(ctx, c, app.WaitOptions{JobID: j.ID, Timeout: timeout, Job: j}))
	})

	flags := cmd.Flags()
	flags.StringVar(&o.name, "name", "", "Job name")
	flags.StringVar(&o.handler, "handler", "", "Handler name that workers claim the job by; required unless the job is pushed")
	flags.StringVar(&o.delay, "delay", "", "Run the job after this long, such as 30s, 10m or 2d")
	flags.StringVar(&o.scheduledFor, "scheduled-for", "", "Run the job at this future time, in RFC 3339")
	flags.StringVar(&o.timeout, "timeout", "", "How long the handler has to finish each attempt, such as 90s or 2m (the API's default is 30s)")
	flags.IntVar(&o.maxRetries, "max-retries", 0, "Retries after a failed attempt (the API's default is 3)")
	flags.StringVar(&o.payload, "payload", "", "JSON object passed to the handler: inline, @path, or @- for stdin")
	flags.StringVar(&o.idempotencyKey, "idempotency-key", "", "Key that makes a repeated create fail instead of making a second job")
	o.push.Register(cmd)
	flags.StringVar(&o.file, "file", "", "Full request body as JSON, from a path or - for stdin")
	o.wait.Register(cmd)
	cmd.MarkFlagsMutuallyExclusive("delay", "scheduled-for")
	return cmd
}

func (o *createFlags) params(flags *pflag.FlagSet, stdin io.Reader) (chronos.JobCreateParams, error) {
	var p chronos.JobCreateParams
	if o.file == "-" && o.payload == "@-" {
		return p, exit.Usagef("--file - and --payload @- both read stdin. Put the payload in the file instead.")
	}
	if o.file != "" {
		if err := input.DecodeFile(o.file, stdin, &p); err != nil {
			return p, err
		}
	}
	if flags.Changed("name") {
		p.Name = o.name
	}
	if flags.Changed("handler") {
		p.Handler = &o.handler
	}
	if flags.Changed("max-retries") {
		p.MaxRetries = &o.maxRetries
	}
	if flags.Changed("idempotency-key") {
		p.IdempotencyKey = &o.idempotencyKey
	}
	if err := o.parseValues(flags, &p, stdin); err != nil {
		return p, err
	}
	push, err := o.push.Parse()
	if err != nil {
		return p, err
	}
	p.Delivery = push.Delivery(p.Delivery)
	return p, nil
}

func (o *createFlags) parseValues(flags *pflag.FlagSet, p *chronos.JobCreateParams, stdin io.Reader) error {
	if flags.Changed("delay") {
		delay, err := input.ParseInterval(o.delay)
		if err != nil {
			return err
		}
		p.Delay, p.ScheduledFor = &delay, nil
	}
	if flags.Changed("scheduled-for") {
		at, err := input.ParseTime(o.scheduledFor)
		if err != nil {
			return err
		}
		p.ScheduledFor, p.Delay = &at, nil
	}
	if flags.Changed("timeout") {
		seconds, err := input.ParseSeconds(o.timeout)
		if err != nil {
			return err
		}
		p.Timeout = &seconds
	}
	if flags.Changed("payload") {
		payload, err := input.ReadPayload(o.payload, stdin)
		if err != nil {
			return err
		}
		p.Payload = payload
	}
	return nil
}
