package monitors_test

import (
	"fmt"
	"maps"
	"strings"
	"testing"

	"chronos.sh/cli/internal/clitest"
)

const monitorID = "0192f3a0-0000-7000-8000-000000000011"

func monitor(t *testing.T, fields map[string]any) string {
	return clitest.FixtureWith(t, "monitor", fields)
}

func newMonitor(t *testing.T, fields map[string]any) string {
	m := map[string]any{"status": "new", "last_ping": nil, "last_started_at": nil, "next_ping_due_at": nil, "alert_at": nil}
	maps.Copy(m, fields)
	return monitor(t, m)
}

func pausedMonitor(t *testing.T) string {
	return monitor(t, map[string]any{"status": "paused", "next_ping_due_at": nil, "alert_at": nil})
}

func ping(t *testing.T, fields map[string]any) string {
	return clitest.FixtureWith(t, "monitor_ping", fields)
}

func failedPing(t *testing.T) string {
	return ping(t, map[string]any{
		"id": "0192f3a0-0000-7000-8000-000000000022", "kind": "fail", "exit_code": 1, "duration_ms": nil,
		"body": "disk full\nretry later", "method": "POST",
	})
}

func run(t *testing.T, api *clitest.API, args ...string) clitest.Result {
	t.Helper()
	return clitest.Run(t, clitest.Options{Args: args, Env: api.Env()})
}

func transcript(api *clitest.API, res clitest.Result) string {
	var b strings.Builder
	for _, r := range api.Requests() {
		fmt.Fprintf(&b, "> %s %s\n", r.Method, r.RequestURI)
		if r.Body != "" {
			fmt.Fprintf(&b, "> %s\n", r.Body)
		}
	}
	fmt.Fprintf(&b, "exit %d\n", res.Code)
	fmt.Fprintf(&b, "--- stdout\n%s", res.Stdout)
	if res.Stderr != "" {
		fmt.Fprintf(&b, "--- stderr\n%s", res.Stderr)
	}
	return strings.ReplaceAll(b.String(), api.URL, "http://api.test")
}

func wantNoRequests(t *testing.T, api *clitest.API) {
	t.Helper()
	if n := len(api.Requests()); n != 0 {
		t.Errorf("the API received %d requests, want 0", n)
	}
}
