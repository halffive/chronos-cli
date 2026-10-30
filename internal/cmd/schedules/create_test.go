package schedules_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/exit"
)

func TestCreate(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "create_pull",
			args: []string{
				"--name", "Nightly report", "--description", "Sends the daily report", "--handler", "report",
				"--cron", "0 3 * * *", "--timezone", "Europe/London", "--runs", "10", "--stop-at", "2027-01-01T00:00:00Z",
				"--timeout", "2m", "--max-retries", "5", "--payload", `{"report":"daily","account_number":9007199254740993}`,
			},
		},
		{
			name: "create_push",
			args: []string{
				"--name", "Sync", "--interval", "5m", "--push-url", "https://example.com/hooks/sync",
				"--push-header", "Authorization: Bearer downstream-secret", "--push-header", "X-Team: ops, platform",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := clitest.NewAPI(t, created(schedule(t, nil)))
			res := run(t, api, append([]string{"--json", "schedules", "create"}, tt.args...)...)
			wantExit(t, res, exit.OK)
			clitest.Golden(t, tt.name, transcript(api, res))
		})
	}
}

func TestCreateFlagsOverrideTheFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "schedule.json")
	body := `{
		"name": "From the file",
		"handler": "report",
		"cron": "0 3 * * *",
		"payload": {"from": "file"},
		"delivery": {"type": "push", "http": {"url": "https://example.com/from-file", "method": "PUT", "headers": {"X-From": "file", "x-team": "file"}}},
		"max_retries": 1
	}`
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	api := clitest.NewAPI(t, created(schedule(t, nil)))
	res := run(t, api, "--json", "schedules", "create", "--file", file,
		"--name", "From flags", "--interval", "1h", "--payload", `{"from":"flags"}`, "--push-header", "X-Team: flags")
	wantExit(t, res, exit.OK)
	clitest.Golden(t, "create_file", transcript(api, res))
}

func TestCreateFromStdin(t *testing.T) {
	api := clitest.NewAPI(t, created(schedule(t, nil)))
	res := clitest.Run(t, clitest.Options{
		Args:  []string{"--json", "schedules", "create", "--file", "-", "--push-url", "https://example.com/hooks/report"},
		Env:   api.Env(),
		Stdin: `{"name":"Nightly report","cron":"0 3 * * *","delivery":{"type":"pull"}}`,
	})
	wantExit(t, res, exit.OK)
	want := `{"name":"Nightly report","cron":"0 3 * * *","delivery":{"type":"push","http":{"url":"https://example.com/hooks/report","method":"POST"}}}`
	if got := api.Requests()[0].Body; got != want {
		t.Errorf("body = %s\nwant   %s", got, want)
	}
}

func TestCreateUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"cron and interval", []string{"--name", "x", "--cron", "* * * * *", "--interval", "5m"}},
		{"stdin read twice", []string{"--file", "-", "--payload", "@-"}},
		{"fractional timeout", []string{"--name", "x", "--timeout", "1.5m"}},
		{"bad interval", []string{"--name", "x", "--interval", "soon"}},
		{"bad stop-at", []string{"--name", "x", "--stop-at", "tomorrow"}},
		{"bad push header", []string{"--name", "x", "--push-header", "no colon"}},
		{"payload not an object", []string{"--name", "x", "--payload", "[1]"}},
		{"unknown field in the file", []string{"--file", "-"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := clitest.NewAPI(t)
			res := clitest.Run(t, clitest.Options{
				Args:  append([]string{"--json", "schedules", "create"}, tt.args...),
				Env:   api.Env(),
				Stdin: `{"name":"x","crn":"* * * * *"}`,
			})
			wantExit(t, res, exit.Usage)
			wantNoRequests(t, api)
		})
	}
}

func TestCreateRejected(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Failure{Status: 400, Code: "handler_required_for_pull", Message: "Pull delivery needs a handler"}.Response())
	res := run(t, api, "--json", "schedules", "create", "--name", "x", "--cron", "* * * * *")
	wantExit(t, res, exit.Rejected)
	if code := jsonError(t, res.Stderr).Code; code != "handler_required_for_pull" {
		t.Errorf("error code = %q", code)
	}
}

func TestRepeatedPushHeaderLastOneWins(t *testing.T) {
	// Map order is random, so a single run could pass by chance.
	for range 20 {
		api := clitest.NewAPI(t, created(schedule(t, nil)))
		res := run(t, api, "--json", "schedules", "create", "--name", "x", "--interval", "5m", "--push-url", "https://example.com/h",
			"--push-header", "X-Token: a", "--push-header", "x-token: b")
		wantExit(t, res, exit.OK)
		if body := api.Requests()[0].Body; !strings.Contains(body, `"headers":{"x-token":"b"}`) {
			t.Fatalf("body = %s", body)
		}
	}
}
