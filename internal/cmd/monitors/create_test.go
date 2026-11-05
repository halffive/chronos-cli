package monitors_test

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
		name     string
		args     []string
		response string
	}{
		{
			name: "create_cron",
			args: []string{
				"--name", "Nightly backup", "--description", "Backs up the database",
				"--cron", "0 3 * * *", "--timezone", "Europe/London", "--grace", "10m",
			},
			response: newMonitor(t, map[string]any{"description": "Backs up the database", "grace_seconds": 600}),
		},
		{
			name:     "create_interval",
			args:     []string{"--name", "Sync", "--interval", "1h"},
			response: newMonitor(t, map[string]any{"name": "Sync", "cron": nil, "interval": map[string]any{"value": 1, "unit": "hour"}}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := clitest.NewAPI(t, clitest.Created(tt.response))
			res := run(t, api, append([]string{"--json", "monitors", "create"}, tt.args...)...)
			clitest.WantExit(t, res, exit.OK)
			clitest.Golden(t, tt.name, transcript(api, res))
		})
	}
}

func TestCreateFlagsOverrideTheFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "monitor.json")
	body := `{"name": "From the file", "cron": "0 3 * * *", "grace_seconds": 120, "description": "From the file"}`
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		flags []string
		want  string
	}{
		{
			[]string{"--name", "From flags", "--interval", "1d"},
			`{"name":"From flags","description":"From the file","interval":{"value":1,"unit":"day"},"grace_seconds":120}`,
		},
		{
			[]string{"--cron", "0 4 * * *", "--grace", "5m"},
			`{"name":"From the file","description":"From the file","cron":"0 4 * * *","grace_seconds":300}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.flags[0], func(t *testing.T) {
			api := clitest.NewAPI(t, clitest.Created(newMonitor(t, nil)))
			res := run(t, api, append([]string{"--json", "monitors", "create", "--file", file}, tt.flags...)...)
			clitest.WantExit(t, res, exit.OK)
			if got := api.Requests()[0].Body; got != tt.want {
				t.Errorf("body = %s\nwant   %s", got, tt.want)
			}
		})
	}
}

func TestCreateFromStdin(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Created(newMonitor(t, nil)))
	res := clitest.Run(t, clitest.Options{
		Args:  []string{"--json", "monitors", "create", "--file", "-", "--grace", "10m"},
		Env:   api.Env(),
		Stdin: `{"name":"Nightly backup","interval":{"value":1,"unit":"day"}}`,
	})
	clitest.WantExit(t, res, exit.OK)
	want := `{"name":"Nightly backup","interval":{"value":1,"unit":"day"},"grace_seconds":600}`
	if got := api.Requests()[0].Body; got != want {
		t.Errorf("body = %s\nwant   %s", got, want)
	}
}

func TestCreatePrintsTheFullPingURL(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Created(newMonitor(t, nil)))
	res := run(t, api, "monitors", "create", "--name", "x", "--interval", "1h")
	clitest.WantExit(t, res, exit.OK)
	if !strings.Contains(res.Stdout, "https://ping.chronos.sh/k3Jx9Qm2TnV7bRw4YcZ8dHfL") {
		t.Errorf("stdout = %s, want the full ping URL", res.Stdout)
	}
}

func TestCreateUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"cron and interval", []string{"--name", "x", "--cron", "* * * * *", "--interval", "5m"}},
		{"fractional grace", []string{"--name", "x", "--cron", "* * * * *", "--grace", "1.5m"}},
		{"bad interval", []string{"--name", "x", "--interval", "soon"}},
		{"unknown field in the file", []string{"--file", "-"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := clitest.NewAPI(t)
			res := clitest.Run(t, clitest.Options{
				Args:  append([]string{"--json", "monitors", "create"}, tt.args...),
				Env:   api.Env(),
				Stdin: `{"name":"x","grace":300}`,
			})
			clitest.WantExit(t, res, exit.Usage)
			wantNoRequests(t, api)
		})
	}
}

func TestCreateAtThePlanLimit(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Failure{Status: 403, Code: "plan_limit_exceeded", Message: "Your plan allows 5 active monitors"}.Response())
	res := run(t, api, "--json", "monitors", "create", "--name", "x", "--interval", "1h")
	clitest.WantExit(t, res, exit.Rejected)
	if code := clitest.DecodeError(t, res.Stderr).Code; code != "plan_limit_exceeded" {
		t.Errorf("error code = %q", code)
	}
}
