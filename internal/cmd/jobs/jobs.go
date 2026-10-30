package jobs

import (
	"context"
	"fmt"
	"strconv"
	"time"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/input"
	"chronos.sh/cli/internal/output"
)

func NewCmd(f *app.Factory) *cobra.Command {
	cmd := cmdutil.NewGroup("jobs", "Create one-off jobs and see what ran")
	cmd.AddCommand(
		newListCmd(f),
		newGetCmd(f),
		newCreateCmd(f),
		newCancelCmd(f),
		newSkipCmd(f),
		newRetryCmd(f),
		newWaitCmd(f),
	)
	return cmd
}

var statuses = []chronos.JobStatus{
	chronos.JobStatusPending, chronos.JobStatusAdmitting, chronos.JobStatusQueued,
	chronos.JobStatusReady, chronos.JobStatusRunning, chronos.JobStatusRetrying,
	chronos.JobStatusCompleted, chronos.JobStatusFailed, chronos.JobStatusCancelled,
	chronos.JobStatusSkipped,
}

var columns = []output.Column[*chronos.Job]{
	{Header: "ID", Value: func(j *chronos.Job) any { return j.ID }},
	{Header: "Name", Value: func(j *chronos.Job) any { return j.Name }, Flex: true},
	{Header: "Status", Value: func(j *chronos.Job) any { return string(j.Status) }, Status: true},
	{Header: "Attempts", Value: attempts},
	{Header: "Scheduled for", Value: func(j *chronos.Job) any { return j.ScheduledFor }},
	{Header: "Completed", Value: func(j *chronos.Job) any { return j.CompletedAt }},
	{Header: "Last error", Value: func(j *chronos.Job) any { return j.LastError }, Flex: true},
}

func attempts(j *chronos.Job) any {
	return strconv.Itoa(j.AttemptCount) + "/" + strconv.Itoa(j.MaxRetries+1)
}

func jobView(j *chronos.Job) output.View {
	return output.Resource(j, jobFields)
}

func jobFields(j *chronos.Job) []output.Field {
	fields := []output.Field{
		{Label: "ID", Value: j.ID},
		{Label: "Name", Value: j.Name},
		{Label: "Status", Value: string(j.Status)},
		{Label: "Origin", Value: string(j.Origin)},
		{Label: "Schedule", Value: j.ScheduleID},
		{Label: "Handler", Value: j.Handler},
		{Label: "Delivery", Value: string(j.Delivery.Type)},
	}
	if h := j.Delivery.HTTP; h != nil {
		fields = append(fields,
			output.Field{Label: "Push to", Value: h.Method + " " + h.URL},
			output.Field{Label: "Push headers", Value: output.MaskedHeaders(h.Headers)},
		)
	}
	var payload any
	if j.Payload != nil {
		payload = output.JSONText(j.Payload)
	}
	return append(fields,
		output.Field{Label: "Payload", Value: payload},
		output.Field{Label: "Attempts", Value: attempts(j)},
		output.Field{Label: "Timeout", Value: strconv.Itoa(j.Timeout) + "s"},
		output.Field{Label: "Idempotency key", Value: j.IdempotencyKey},
		output.Field{Label: "Scheduled for", Value: j.ScheduledFor},
		output.Field{Label: "Run at", Value: j.RunAt},
		output.Field{Label: "Started", Value: j.StartedAt},
		output.Field{Label: "Completed", Value: j.CompletedAt},
		output.Field{Label: "Last error", Value: j.LastError},
		output.Field{Label: "Created", Value: j.CreatedAt},
		output.Field{Label: "Updated", Value: j.UpdatedAt},
	)
}

func addWaitTimeoutFlag(cmd *cobra.Command, value *string) {
	cmd.Flags().StringVar(value, "wait-timeout", fmt.Sprintf("%ds", int(app.DefaultWaitTimeout.Seconds())), "How long to wait for the job to finish, such as 90s or 5m")
}

func parseWaitTimeout(s string) (time.Duration, error) {
	seconds, err := input.ParseSeconds(s)
	if err != nil {
		return 0, err
	}
	timeout := time.Duration(seconds) * time.Second
	return timeout, app.CheckWaitTimeout(timeout)
}

func waitView(j *chronos.Job, err error) (output.View, error) {
	if j == nil {
		return nil, err
	}
	return jobView(j), err
}

type jobAction func(s *chronos.JobsService, ctx context.Context, id string) (*chronos.Job, error)

func newActionCmd(f *app.Factory, spec cmdutil.CommandSpec, action jobAction) *cobra.Command {
	spec.Args = cmdutil.ExactArgs("id")
	spec.Mutates = true
	return cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, args []string) (output.View, error) {
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		j, err := action(c.Jobs, ctx, args[0])
		if err != nil {
			return nil, err
		}
		return jobView(j), nil
	})
}
