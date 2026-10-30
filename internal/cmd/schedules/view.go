package schedules

import (
	"fmt"
	"strconv"

	chronos "chronos.sh/go"

	"chronos.sh/cli/internal/input"
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
			output.Field{Label: "Push URL", Value: output.RedactedURL(h.URL)},
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

func cadence(s *chronos.Schedule) string {
	if s.Cron != nil {
		return fmt.Sprintf("%s (%s)", *s.Cron, s.Timezone)
	}
	if s.Interval == nil {
		return ""
	}
	return "every " + input.FormatInterval(*s.Interval)
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
