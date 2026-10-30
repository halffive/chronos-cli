package executions

import (
	"time"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

func NewCmd(f *app.Factory) *cobra.Command {
	cmd := cmdutil.NewGroup("executions", "See each attempt to run a job")
	cmd.AddCommand(newListCmd(f), newGetCmd(f))
	return cmd
}

var statuses = []chronos.ExecutionStatus{
	chronos.ExecutionStatusPending, chronos.ExecutionStatusRunning, chronos.ExecutionStatusCompleted,
	chronos.ExecutionStatusFailed, chronos.ExecutionStatusTimeout,
}

var columns = []output.Column[*chronos.Execution]{
	{Header: "ID", Value: func(e *chronos.Execution) any { return e.ID }},
	{Header: "Job", Value: func(e *chronos.Execution) any { return e.JobID }},
	{Header: "Status", Value: func(e *chronos.Execution) any { return string(e.Status) }, Status: true},
	{Header: "Trigger", Value: func(e *chronos.Execution) any { return string(e.Trigger) }},
	{Header: "Duration", Value: duration},
	{Header: "Response code", Value: func(e *chronos.Execution) any { return e.ResponseCode }},
	{Header: "Started", Value: func(e *chronos.Execution) any { return e.StartedAt }},
	{Header: "Error", Value: func(e *chronos.Execution) any { return e.Error }, Flex: true},
}

func duration(e *chronos.Execution) any {
	if e.DurationMS == nil {
		return nil
	}
	return (time.Duration(*e.DurationMS) * time.Millisecond).String()
}

func executionView(e *chronos.Execution) output.View {
	return output.Resource(e, executionFields)
}

func executionFields(e *chronos.Execution) []output.Field {
	var result any
	if e.Result != nil {
		result = output.JSONText(e.Result)
	}
	return []output.Field{
		{Label: "ID", Value: e.ID},
		{Label: "Job", Value: e.JobID},
		{Label: "Status", Value: string(e.Status)},
		{Label: "Trigger", Value: string(e.Trigger)},
		{Label: "Response code", Value: e.ResponseCode},
		{Label: "Duration", Value: duration(e)},
		{Label: "Started", Value: e.StartedAt},
		{Label: "Completed", Value: e.CompletedAt},
		{Label: "Created", Value: e.CreatedAt},
		{Label: "Error", Value: e.Error},
		{Label: "Result", Value: result},
	}
}
