package schedules

import (
	"fmt"
	"strconv"

	chronos "chronos.sh/go"

	"chronos.sh/cli/internal/output"
)

var columns = []output.Column[*chronos.Schedule]{
	{Header: "ID", Value: func(s *chronos.Schedule) any { return s.ID }},
	{Header: "Name", Value: func(s *chronos.Schedule) any { return s.Name }, Flex: true},
	{Header: "Status", Value: func(s *chronos.Schedule) any { return string(s.Status) }, Status: true},
	{Header: "Cadence", Value: func(s *chronos.Schedule) any { return cadence(s) }},
	{Header: "Next run", Value: func(s *chronos.Schedule) any { return s.NextRunAt }},
	{Header: "Last run", Value: func(s *chronos.Schedule) any { return s.LastRunAt }},
	{Header: "Delivery", Value: func(s *chronos.Schedule) any { return string(s.Delivery.Type) }},
}

func scheduleView(s *chronos.Schedule) output.View {
	return output.Resource(s, scheduleFields)
}

func scheduleFields(s *chronos.Schedule) []output.Field {
	fields := []output.Field{
		{Label: "ID", Value: s.ID},
		{Label: "Name", Value: s.Name},
		{Label: "Status", Value: string(s.Status)},
		{Label: "Description", Value: s.Description},
		{Label: "Cadence", Value: cadence(s)},
		{Label: "Handler", Value: s.Handler},
		{Label: "Runs", Value: s.Runs},
		{Label: "Run count", Value: s.RunCount},
		{Label: "Stop at", Value: s.StopAt},
		{Label: "Timeout", Value: seconds(s.Timeout)},
		{Label: "Max retries", Value: s.MaxRetries},
		{Label: "Delivery", Value: string(s.Delivery.Type)},
	}
	if h := s.Delivery.HTTP; h != nil {
		fields = append(fields,
			output.Field{Label: "Push URL", Value: h.URL},
			output.Field{Label: "Push method", Value: h.Method},
			output.Field{Label: "Push headers", Value: output.MaskedHeaders(h.Headers)},
		)
	}
	return append(fields,
		output.Field{Label: "Payload", Value: payloadText(s.Payload)},
		output.Field{Label: "Next run", Value: s.NextRunAt},
		output.Field{Label: "Last run", Value: s.LastRunAt},
		output.Field{Label: "Created", Value: s.CreatedAt},
		output.Field{Label: "Updated", Value: s.UpdatedAt},
	)
}

var unitSuffixes = map[chronos.IntervalUnit]string{
	chronos.IntervalUnitSecond: "s",
	chronos.IntervalUnitMinute: "m",
	chronos.IntervalUnitHour:   "h",
	chronos.IntervalUnitDay:    "d",
	chronos.IntervalUnitWeek:   "w",
}

func cadence(s *chronos.Schedule) string {
	if s.Cron != nil {
		return fmt.Sprintf("%s (%s)", *s.Cron, s.Timezone)
	}
	if s.Interval == nil {
		return ""
	}
	if suffix, ok := unitSuffixes[s.Interval.Unit]; ok {
		return "every " + strconv.Itoa(s.Interval.Value) + suffix
	}
	return fmt.Sprintf("every %d %s", s.Interval.Value, s.Interval.Unit)
}

func seconds(n int) string {
	return strconv.Itoa(n) + "s"
}

func payloadText(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	return output.JSONText(payload)
}

func jobView(j *chronos.Job) output.View {
	return output.Resource(j, func(j *chronos.Job) []output.Field {
		return []output.Field{
			{Label: "ID", Value: j.ID},
			{Label: "Name", Value: j.Name},
			{Label: "Status", Value: string(j.Status)},
			{Label: "Schedule ID", Value: j.ScheduleID},
			{Label: "Scheduled for", Value: j.ScheduledFor},
			{Label: "Attempts", Value: j.AttemptCount},
			{Label: "Last error", Value: j.LastError},
			{Label: "Started", Value: j.StartedAt},
			{Label: "Completed", Value: j.CompletedAt},
		}
	})
}
