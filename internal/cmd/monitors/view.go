package monitors

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	chronos "chronos.sh/go"

	"chronos.sh/cli/internal/input"
	"chronos.sh/cli/internal/output"
)

var columns = []output.Column[*chronos.Monitor]{
	{Header: "ID", Value: func(m *chronos.Monitor) any { return m.ID }},
	{Header: "Name", Value: func(m *chronos.Monitor) any { return m.Name }, Flex: true},
	{Header: "Status", Value: func(m *chronos.Monitor) any { return string(m.Status) }, Status: true},
	{Header: "Cadence", Value: func(m *chronos.Monitor) any { return cadence(m) }},
	{Header: "Last ping", Value: func(m *chronos.Monitor) any { return lastPingKind(m.LastPing) }},
	{Header: "Last ping at", Value: func(m *chronos.Monitor) any { return lastPingAt(m.LastPing) }},
	{Header: "Next ping due", Value: func(m *chronos.Monitor) any { return m.NextPingDueAt }},
}

var pingColumns = []output.Column[*chronos.MonitorPing]{
	{Header: "Received", Value: func(p *chronos.MonitorPing) any { return p.ReceivedAt }},
	{Header: "Kind", Value: func(p *chronos.MonitorPing) any { return string(p.Kind) }},
	{Header: "Exit code", Value: func(p *chronos.MonitorPing) any { return p.ExitCode }},
	{Header: "Duration", Value: func(p *chronos.MonitorPing) any { return duration(p.DurationMS) }},
	{Header: "Method", Value: func(p *chronos.MonitorPing) any { return p.Method }},
	{Header: "User agent", Value: func(p *chronos.MonitorPing) any { return p.UserAgent }, Flex: true},
	{Header: "Body", Value: func(p *chronos.MonitorPing) any { return p.Body }, Flex: true},
}

func monitorView(m *chronos.Monitor, pingURL func(string) string) output.View {
	return output.Resource(m, func(m *chronos.Monitor) []output.Field { return monitorFields(m, pingURL(m.PingURL)) })
}

func monitorFields(m *chronos.Monitor, pingURL string) []output.Field {
	return []output.Field{
		{Label: "ID", Value: m.ID},
		{Label: "Name", Value: m.Name},
		{Label: "Status", Value: string(m.Status)},
		{Label: "Description", Value: m.Description},
		{Label: "Cadence", Value: cadence(m)},
		{Label: "Grace", Value: strconv.Itoa(m.GraceSeconds) + "s"},
		{Label: "Ping URL", Value: pingURL},
		{Label: "Last ping", Value: lastPingText(m.LastPing)},
		{Label: "Last ping at", Value: lastPingAt(m.LastPing)},
		{Label: "Last started", Value: m.LastStartedAt},
		{Label: "Next ping due", Value: m.NextPingDueAt},
		{Label: "Alert at", Value: m.AlertAt},
		{Label: "Created", Value: m.CreatedAt},
		{Label: "Updated", Value: m.UpdatedAt},
	}
}

const hiddenToken = "****"

func fullPingURL(s string) string {
	if strings.HasSuffix(s, "/"+hiddenToken) {
		return s + " (ping token hidden, since the login or API key in use lacks monitors:write)"
	}
	return s
}

func maskedPingURL(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return output.Masked
	}
	return u.Scheme + "://" + u.Host + "/" + output.Masked
}

func cadence(m *chronos.Monitor) string {
	if m.Cron != nil {
		return fmt.Sprintf("%s (%s)", *m.Cron, m.Timezone)
	}
	if m.Interval == nil {
		return ""
	}
	return "every " + input.FormatInterval(*m.Interval)
}

func lastPingKind(p *chronos.MonitorPingSummary) string {
	if p == nil {
		return ""
	}
	return string(p.Kind)
}

func lastPingText(p *chronos.MonitorPingSummary) string {
	if p == nil || p.ExitCode == nil {
		return lastPingKind(p)
	}
	return fmt.Sprintf("%s (exit %d)", p.Kind, *p.ExitCode)
}

func lastPingAt(p *chronos.MonitorPingSummary) *time.Time {
	if p == nil {
		return nil
	}
	return &p.ReceivedAt
}

func duration(ms *int) any {
	if ms == nil {
		return nil
	}
	return (time.Duration(*ms) * time.Millisecond).String()
}
