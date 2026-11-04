package monitors_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/exit"
)

func TestUpdate(t *testing.T) {
	api := clitest.NewAPI(t, clitest.OK(monitor(t, nil)))
	res := run(t, api, "--json", "monitors", "update", monitorID,
		"--name", "Nightly backup", "--description", "Backs up the database",
		"--cron", "0 4 * * *", "--timezone", "Europe/Paris", "--grace", "900")
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "update", transcript(api, res))
}

func TestUpdateFileFlagsAndClear(t *testing.T) {
	file := filepath.Join(t.TempDir(), "update.json")
	body := `{"name": "From the file", "interval": {"value": 1, "unit": "hour"}, "description": "From the file"}`
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	api := clitest.NewAPI(t, clitest.OK(monitor(t, nil)))
	res := run(t, api, "--json", "monitors", "update", monitorID, "--file", file, "--cron", "0 4 * * *", "--clear", "description")
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "update_file", transcript(api, res))
}

func TestUpdateWithNoFieldsIsLeftToTheAPI(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Failure{
		Status: 422, Code: "validation_error", Message: "request body failed validation",
		FieldErrors: `[{"message":"at least one field must be provided","field":null}]`,
	}.Response())
	res := run(t, api, "--json", "monitors", "update", monitorID)
	clitest.WantExit(t, res, exit.Rejected)
	if got := api.Requests()[0].Body; got != "{}" {
		t.Errorf("body = %s, want {}", got)
	}
}

func TestUpdateHidesThePingURLToken(t *testing.T) {
	api := clitest.NewAPI(t, clitest.OK(monitor(t, nil)))
	res := run(t, api, "monitors", "update", monitorID, "--grace", "10m")
	clitest.WantExit(t, res, exit.OK)
	if strings.Contains(res.Stdout, "k3Jx9Qm2TnV7bRw4YcZ8dHfL") || !strings.Contains(res.Stdout, "https://ping.chronos.sh/••••") {
		t.Errorf("stdout = %s, want the ping URL with its token hidden", res.Stdout)
	}
}

func TestUpdateClearDescription(t *testing.T) {
	api := clitest.NewAPI(t, clitest.OK(monitor(t, nil)))
	res := run(t, api, "--json", "monitors", "update", monitorID, "--interval", "2h", "--clear", "description")
	clitest.WantExit(t, res, exit.OK)
	want := `{"description":null,"interval":{"value":2,"unit":"hour"}}`
	if got := api.Requests()[0].Body; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestUpdateClearUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"set and clear", []string{"--description", "x", "--clear", "description"}},
		{"field that cannot be cleared", []string{"--clear", "grace"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := clitest.NewAPI(t)
			res := run(t, api, append([]string{"--json", "monitors", "update", monitorID}, tt.args...)...)
			clitest.WantExit(t, res, exit.Usage)
			wantNoRequests(t, api)
		})
	}
}
