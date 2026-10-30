package executions_test

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	chronos "chronos.sh/go"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/exit"
)

const executionID = "0192f3a3-0000-7000-8000-000000000005"

func execution(t *testing.T, changes map[string]any) string {
	t.Helper()
	var e map[string]any
	if err := json.Unmarshal([]byte(clitest.Fixture("execution")), &e); err != nil {
		t.Fatal(err)
	}
	for k, v := range changes {
		e[k] = v
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func timedOut(t *testing.T) string {
	return execution(t, map[string]any{
		"id":            "0192f3a3-0000-7000-8000-000000000005",
		"status":        "timeout",
		"trigger":       "system_retry",
		"duration_ms":   30000,
		"response_code": nil,
		"error":         "Handler did not respond within 30s.\nThe connection was closed after the timeout and the attempt counted as failed.",
		"result":        map[string]any{"rows": 9007199254740993, "note": "partial"},
	})
}

func run(t *testing.T, env map[string]string, args ...string) clitest.Result {
	t.Helper()
	return clitest.Run(t, clitest.Options{Args: args, Env: env})
}

func terminal(t *testing.T, api *clitest.API, args ...string) clitest.Result {
	t.Helper()
	env := api.Env()
	env["NO_COLOR"] = "1"
	return clitest.Run(t, clitest.Options{Args: args, Env: env, StdoutTTY: true, Width: 150})
}

func wantExit(t *testing.T, res clitest.Result, want exit.Code) {
	t.Helper()
	if res.Code != want {
		t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", res.Code, want, res.Stdout, res.Stderr)
	}
}

func query(t *testing.T, req clitest.Request) url.Values {
	t.Helper()
	u, err := url.Parse(req.RequestURI)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

func TestListJSON(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Page("["+timedOut(t)+"]", chronos.Paging{Page: 1, Pages: 1, Size: 20, Total: 1}))
	res := run(t, api.Env(), "--json", "executions", "list",
		"--job-id", "0192f3a2-0000-7000-8000-000000000003", "--schedule-id", "null", "--handler", "report",
		"--status", "timeout", "--created-after", "1h", "--created-before", "2026-10-28T11:30:00Z", "--size", "50")
	wantExit(t, res, exit.OK)
	clitest.Golden(t, "list", res.Stdout)
	want := url.Values{
		"job_id":         {"0192f3a2-0000-7000-8000-000000000003"},
		"schedule_id":    {"null"},
		"handler":        {"report"},
		"status":         {"timeout"},
		"created_after":  {"2026-10-28T11:00:00Z"},
		"created_before": {"2026-10-28T11:30:00Z"},
		"size":           {"50"},
	}
	if got := query(t, api.Requests()[0]); got.Encode() != want.Encode() {
		t.Errorf("query = %s\nwant    %s", got.Encode(), want.Encode())
	}
}

func TestListByScheduleID(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Page("[]", chronos.Paging{Page: 1, Pages: 0, Size: 20, Total: 0}))
	res := run(t, api.Env(), "--json", "executions", "list", "--schedule-id", "0192f3a0-0000-7000-8000-000000000001")
	wantExit(t, res, exit.OK)
	if got := query(t, api.Requests()[0]).Get("schedule_id"); got != "0192f3a0-0000-7000-8000-000000000001" {
		t.Errorf("schedule_id = %q", got)
	}
}

func TestListAllWalksEveryPage(t *testing.T) {
	api := clitest.NewAPI(t,
		clitest.Page("["+clitest.Fixture("execution")+"]", chronos.Paging{Page: 1, Pages: 2, Size: 1, Total: 2}),
		clitest.Page("["+execution(t, map[string]any{"id": "0192f3a3-0000-7000-8000-000000000006"})+"]", chronos.Paging{Page: 2, Pages: 2, Size: 1, Total: 2}),
	)
	res := run(t, api.Env(), "--json", "executions", "list", "--status", "failed", "--all", "--size", "1")
	wantExit(t, res, exit.OK)
	clitest.Golden(t, "list_all", res.Stdout)
	if got := query(t, api.Requests()[1]).Encode(); got != "page=2&size=1&status=failed" {
		t.Errorf("second page query = %s", got)
	}
}

func TestListTable(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Page("["+clitest.Fixture("execution")+","+timedOut(t)+"]", chronos.Paging{Page: 1, Pages: 1, Size: 20, Total: 2}))
	res := terminal(t, api, "executions", "list")
	wantExit(t, res, exit.OK)
	clitest.Golden(t, "list_table", res.Stdout)
}

func TestGet(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		api := clitest.NewAPI(t, clitest.OK(timedOut(t)))
		res := run(t, api.Env(), "--json", "executions", "get", executionID)
		wantExit(t, res, exit.OK)
		clitest.Golden(t, "get", res.Stdout)
		if got := api.Requests()[0].RequestURI; got != "/v1/executions/"+executionID {
			t.Errorf("request = %s", got)
		}
	})
	t.Run("terminal shows the full error and result", func(t *testing.T) {
		api := clitest.NewAPI(t, clitest.OK(timedOut(t)))
		res := terminal(t, api, "executions", "get", executionID)
		wantExit(t, res, exit.OK)
		clitest.Golden(t, "get_terminal", res.Stdout)
		if !strings.Contains(res.Stdout, "counted as failed.") || !strings.Contains(res.Stdout, "9007199254740993") {
			t.Errorf("stdout = %s", res.Stdout)
		}
	})
	t.Run("not found", func(t *testing.T) {
		api := clitest.NewAPI(t, clitest.Failure{Status: 404, Code: "execution_not_found", Message: "Execution not found"}.Response())
		res := run(t, api.Env(), "--json", "executions", "get", executionID)
		wantExit(t, res, exit.NotFound)
		var e struct{ Error exit.Outcome }
		if err := json.Unmarshal([]byte(res.Stderr), &e); err != nil || e.Error.Code != "execution_not_found" {
			t.Errorf("stderr = %s", res.Stderr)
		}
	})
}

func TestListUsageErrors(t *testing.T) {
	for name, args := range map[string][]string{
		"bad time filter": {"--created-after", "yesterday"},
		"page with all":   {"--page", "2", "--all"},
	} {
		t.Run(name, func(t *testing.T) {
			api := clitest.NewAPI(t)
			res := run(t, api.Env(), append([]string{"--json", "executions", "list"}, args...)...)
			wantExit(t, res, exit.Usage)
		})
	}
}
