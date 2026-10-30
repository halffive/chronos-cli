package schedules_test

import (
	"fmt"
	"strings"
	"testing"

	"chronos.sh/cli/internal/clitest"
)

const scheduleID = "0192f3a0-0000-7000-8000-000000000001"

func schedule(t *testing.T, fields map[string]any) string {
	return clitest.FixtureWith(t, "schedule", fields)
}

func job(t *testing.T, status string) string {
	return clitest.FixtureWith(t, "job", map[string]any{"status": status})
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
