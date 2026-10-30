package schedules_test

import (
	"os"
	"path/filepath"
	"testing"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/exit"
)

func TestUpdate(t *testing.T) {
	api := clitest.NewAPI(t, clitest.OK(schedule(t, nil)))
	res := run(t, api, "--json", "schedules", "update", scheduleID,
		"--name", "Nightly report", "--description", "Sends the daily report", "--handler", "report",
		"--cron", "0 4 * * *", "--timezone", "Europe/Paris", "--runs", "10", "--stop-at", "2027-01-01T00:00:00Z",
		"--timeout", "45", "--max-retries", "0", "--payload", `{"account_number":9007199254740993}`,
		"--push-url", "https://example.com/hooks/report", "--push-method", "PUT")
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "update", transcript(api, res))
}

func TestUpdateFileFlagsAndClear(t *testing.T) {
	file := filepath.Join(t.TempDir(), "update.json")
	body := `{"name": "From the file", "interval": {"value": 1, "unit": "hour"}, "runs": 5, "description": "From the file"}`
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	api := clitest.NewAPI(t, clitest.OK(schedule(t, nil)))
	res := run(t, api, "--json", "schedules", "update", scheduleID, "--file", file, "--cron", "0 4 * * *", "--clear", "runs")
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "update_file", transcript(api, res))
}

func TestUpdateClear(t *testing.T) {
	tests := []struct {
		field string
		body  string
	}{
		{"description", `{"description":null}`},
		{"handler", `{"handler":null}`},
		{"runs", `{"runs":null}`},
		{"stop-at", `{"stop_at":null}`},
		{"payload", `{"payload":null}`},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			api := clitest.NewAPI(t, clitest.OK(schedule(t, nil)))
			res := run(t, api, "--json", "schedules", "update", scheduleID, "--clear", tt.field)
			clitest.WantExit(t, res, exit.OK)
			if got := api.Requests()[0].Body; got != tt.body {
				t.Errorf("body = %s, want %s", got, tt.body)
			}
		})
	}
}

func TestUpdateClearUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"set and clear", []string{"--runs", "5", "--clear", "runs"}},
		{"set and clear in one list", []string{"--stop-at", "2027-01-01T00:00:00Z", "--clear", "description,stop-at"}},
		{"field that cannot be cleared", []string{"--clear", "name"}},
		{"API spelling", []string{"--clear", "stop_at"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := clitest.NewAPI(t)
			res := run(t, api, append([]string{"--json", "schedules", "update", scheduleID}, tt.args...)...)
			clitest.WantExit(t, res, exit.Usage)
			wantNoRequests(t, api)
		})
	}
}

func TestUpdateWithNoFieldsIsLeftToTheAPI(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Failure{
		Status: 422, Code: "validation_error", Message: "request body failed validation",
		FieldErrors: `[{"message":"at least one field must be provided","field":null}]`,
	}.Response())
	res := run(t, api, "--json", "schedules", "update", scheduleID)
	clitest.WantExit(t, res, exit.Rejected)
	if got := api.Requests()[0].Body; got != "{}" {
		t.Errorf("body = %s, want {}", got)
	}
}

func TestUpdateFileKeepsLargeIntegers(t *testing.T) {
	file := filepath.Join(t.TempDir(), "update.json")
	body := `{"payload":{"n":9007199254740993}}`
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	api := clitest.NewAPI(t, clitest.OK(schedule(t, nil)))
	res := run(t, api, "--json", "schedules", "update", scheduleID, "--file", file)
	clitest.WantExit(t, res, exit.OK)
	if got := api.Requests()[0].Body; got != body {
		t.Errorf("body = %s, want %s", got, body)
	}
}
