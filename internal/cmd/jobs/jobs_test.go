package jobs_test

import (
	"encoding/json"
	"strings"
	"testing"

	chronos "chronos.sh/go"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/exit"
)

const jobID = "0192f3a2-0000-7000-8000-000000000003"

func job(t *testing.T, changes map[string]any) string {
	return clitest.FixtureWith(t, "job", changes)
}

func withStatus(t *testing.T, status string) string {
	return job(t, map[string]any{"status": status})
}

func failedJob(t *testing.T) string {
	return job(t, map[string]any{
		"status":        "failed",
		"attempt_count": 4,
		"max_retries":   3,
		"completed_at":  "2026-10-28T11:30:00Z",
		"last_error":    "HTTP 503 from https://example.com/hooks/report after 30s: upstream connect error or disconnect/reset before headers",
	})
}

func run(t *testing.T, api *clitest.API, args ...string) clitest.Result {
	t.Helper()
	return clitest.Run(t, clitest.Options{Args: args, Env: api.Env()})
}

func noColor(env map[string]string) map[string]string {
	env["NO_COLOR"] = "1"
	return env
}

func wantQuery(t *testing.T, req clitest.Request, want map[string]string) {
	t.Helper()
	got := map[string]string{}
	for k, v := range clitest.Query(t, req) {
		got[k] = strings.Join(v, ",")
	}
	if len(got) != len(want) {
		t.Errorf("query = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("query %s = %q, want %q", k, got[k], v)
		}
	}
}

func TestListJSON(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Page("["+failedJob(t)+"]", chronos.Paging{Page: 2, Pages: 3, Size: 5, Total: 11}))
	res := run(t, api, "--json", "jobs", "list",
		"--status", "failed", "--origin", "schedule", "--schedule-id", "0192f3a0-0000-7000-8000-000000000001",
		"--handler", "report", "--search", "nightly",
		"--scheduled-after", "24h", "--scheduled-before", "2026-10-28T11:00:00+01:00",
		"--page", "2", "--size", "5")
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "list", res.Stdout)
	wantQuery(t, api.Requests()[0], map[string]string{
		"status": "failed", "origin": "schedule", "schedule_id": "0192f3a0-0000-7000-8000-000000000001",
		"handler": "report", "search": "nightly",
		"scheduled_after": "2026-10-27T12:00:00Z", "scheduled_before": "2026-10-28T10:00:00Z",
		"page": "2", "size": "5",
	})
}

func TestListAllWalksEveryPage(t *testing.T) {
	api := clitest.NewAPI(t,
		clitest.Page("["+withStatus(t, "failed")+"]", chronos.Paging{Page: 1, Pages: 2, Size: 1, Total: 2}),
		clitest.Page("["+job(t, map[string]any{"id": "0192f3a2-0000-7000-8000-000000000005", "status": "failed"})+"]", chronos.Paging{Page: 2, Pages: 2, Size: 1, Total: 2}),
	)
	res := run(t, api, "--json", "jobs", "list", "--status", "failed", "--all", "--size", "1")
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "list_all", res.Stdout)
	reqs := api.Requests()
	if len(reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(reqs))
	}
	wantQuery(t, reqs[1], map[string]string{"status": "failed", "page": "2", "size": "1"})
}

func TestListTable(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Page("["+failedJob(t)+"]", chronos.Paging{Page: 1, Pages: 1, Size: 20, Total: 1}))
	res := clitest.Run(t, clitest.Options{Args: []string{"jobs", "list"}, Env: noColor(api.Env()), StdoutTTY: true, Width: 150})
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "list_table", res.Stdout)
}

func TestGet(t *testing.T) {
	pushed := job(t, map[string]any{
		"delivery": map[string]any{"type": "push", "http": map[string]any{
			"url": "https://example.com/hooks/report", "method": "POST",
			"headers": map[string]string{"Authorization": "Bearer downstream-secret"},
		}},
	})
	t.Run("json", func(t *testing.T) {
		api := clitest.NewAPI(t, clitest.OK(pushed))
		res := run(t, api, "--json", "jobs", "get", jobID)
		clitest.WantExit(t, res, exit.OK)
		clitest.Golden(t, "get", res.Stdout)
		if got := api.Requests()[0].RequestURI; got != "/v1/jobs/"+jobID {
			t.Errorf("request = %s", got)
		}
	})
	t.Run("terminal", func(t *testing.T) {
		api := clitest.NewAPI(t, clitest.OK(pushed))
		res := clitest.Run(t, clitest.Options{Args: []string{"jobs", "get", jobID}, Env: noColor(api.Env()), StdoutTTY: true})
		clitest.WantExit(t, res, exit.OK)
		clitest.Golden(t, "get_terminal", res.Stdout)
		if strings.Contains(res.Stdout, "downstream-secret") {
			t.Error("a push header value was printed")
		}
	})
}

func TestCreate(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		stdin string
	}{
		{
			name: "flags",
			args: []string{
				"--name", "Report", "--delay", "10m", "--timeout", "2m", "--max-retries", "0",
				"--payload", `{"account_number":9007199254740993}`, "--idempotency-key", "report-2026-10-28",
				"--push-url", "https://example.com/hooks/report", "--push-header", "Authorization: Bearer downstream-secret",
			},
		},
		{
			name:  "file with overrides",
			args:  []string{"--file", "-", "--name", "Report", "--push-method", "PUT", "--push-header", "X-Team: ops"},
			stdin: `{"name":"From file","scheduled_for":"2026-11-01T09:00:00Z","delivery":{"type":"push","http":{"url":"https://example.com/hooks/report","method":"POST","headers":{"Authorization":"Bearer a"}}},"payload":{"n":1}}`,
		},
		{
			name:  "file timing replaced by flag",
			args:  []string{"--file", "-", "--delay", "5m"},
			stdin: `{"name":"From file","handler":"report","scheduled_for":"2026-11-01T09:00:00Z"}`,
		},
		{
			name:  "file method kept and header replaced",
			args:  []string{"--file", "-", "--push-header", "Authorization: Bearer new"},
			stdin: `{"name":"From file","delivery":{"type":"push","http":{"url":"https://example.com/hooks/report","method":"PUT","headers":{"authorization":"Bearer old"}}}}`,
		},
		{
			name: "pull now",
			args: []string{"--name", "Report", "--handler", "send-report"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := clitest.NewAPI(t, clitest.Created(clitest.Fixture("job")))
			res := clitest.Run(t, clitest.Options{Args: append([]string{"--json", "jobs", "create"}, tt.args...), Env: api.Env(), Stdin: tt.stdin})
			clitest.WantExit(t, res, exit.OK)
			clitest.Golden(t, "create", res.Stdout)
			req := api.Requests()[0]
			if req.Method != "POST" || req.RequestURI != "/v1/jobs" {
				t.Errorf("request = %s %s", req.Method, req.RequestURI)
			}
			clitest.Golden(t, "create_body_"+strings.ReplaceAll(tt.name, " ", "_"), req.Body+"\n")
		})
	}
}

func TestCreateUsageErrorsSendNothing(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"wait timeout below a second", []string{"--name", "Report", "--wait", "--wait-timeout", "0"}},
		{"wait timeout without wait", []string{"--name", "Report", "--wait-timeout", "5m"}},
		{"fractional timeout", []string{"--name", "Report", "--timeout", "1.5m"}},
		{"bad delay", []string{"--name", "Report", "--delay", "soon"}},
		{"bad scheduled-for", []string{"--name", "Report", "--scheduled-for", "tomorrow"}},
		{"bad push header", []string{"--name", "Report", "--push-url", "https://example.com", "--push-header", "nocolon"}},
		{"delay and scheduled-for", []string{"--name", "Report", "--delay", "5m", "--scheduled-for", "2026-11-01T09:00:00Z"}},
		{"stdin read twice", []string{"--file", "-", "--payload", "@-"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := clitest.NewAPI(t)
			res := run(t, api, append([]string{"--json", "jobs", "create"}, tt.args...)...)
			clitest.WantExit(t, res, exit.Usage)
		})
	}
}

func TestWait(t *testing.T) {
	tests := []struct {
		status string
		exit   exit.Code
	}{
		{"completed", exit.OK},
		{"failed", exit.JobUnsuccessful},
		{"cancelled", exit.JobUnsuccessful},
		{"skipped", exit.JobUnsuccessful},
		{"running", exit.WaitTimeout},
	}
	commands := map[string][]string{
		"jobs wait":          {"jobs", "wait", jobID, "--wait-timeout", "1s"},
		"jobs create --wait": {"jobs", "create", "--name", "Report", "--handler", "report", "--wait", "--wait-timeout", "1s"},
	}
	for name, args := range commands {
		for _, tt := range tests {
			t.Run(name+" "+tt.status, func(t *testing.T) {
				t.Parallel()
				responses := []clitest.Response{clitest.OK(withStatus(t, "running")), clitest.OK(withStatus(t, tt.status))}
				if args[1] == "create" {
					responses = append([]clitest.Response{clitest.Created(withStatus(t, "queued"))}, responses...)
				}
				api := clitest.NewAPI(t, responses...)
				res := run(t, api, append([]string{"--json"}, args...)...)
				clitest.WantExit(t, res, tt.exit)
				var got chronos.Job
				if err := json.Unmarshal([]byte(res.Stdout), &got); err != nil || string(got.Status) != tt.status {
					t.Errorf("stdout = %s, want the job with status %s", res.Stdout, tt.status)
				}
			})
		}
	}
}

func TestWaitJSON(t *testing.T) {
	api := clitest.NewAPI(t, clitest.OK(failedJob(t)))
	res := run(t, api, "--json", "jobs", "wait", jobID)
	clitest.WantExit(t, res, exit.JobUnsuccessful)
	clitest.Golden(t, "wait", res.Stdout)
	clitest.Golden(t, "wait_error", res.Stderr)
}

func TestActions(t *testing.T) {
	for _, verb := range []string{"cancel", "skip", "retry"} {
		t.Run(verb, func(t *testing.T) {
			api := clitest.NewAPI(t, clitest.OK(withStatus(t, "cancelled")))
			res := run(t, api, "--json", "jobs", verb, jobID)
			clitest.WantExit(t, res, exit.OK)
			clitest.Golden(t, verb, res.Stdout)
			req := api.Requests()[0]
			if req.Method != "POST" || req.RequestURI != "/v1/jobs/"+jobID+"/"+verb {
				t.Errorf("request = %s %s", req.Method, req.RequestURI)
			}
		})
	}
}

func TestDryRunSendsNothing(t *testing.T) {
	tests := map[string][]string{
		"create": {"create", "--name", "Report", "--push-url", "https://example.com/hooks/report", "--push-header", "Authorization: Bearer downstream-secret", "--wait"},
		"cancel": {"cancel", jobID},
		"skip":   {"skip", jobID},
		"retry":  {"retry", jobID},
	}
	for verb, args := range tests {
		t.Run(verb, func(t *testing.T) {
			api := clitest.NewAPI(t)
			res := clitest.Run(t, clitest.Options{
				Args: append(append([]string{"--json", "jobs"}, args...), "--dry-run"),
				Env:  map[string]string{"CHRONOS_BASE_URL": api.URL},
			})
			clitest.WantExit(t, res, exit.OK)
			clitest.Golden(t, "dry_run_"+verb, strings.ReplaceAll(res.Stdout, api.URL, "http://api.test"))
		})
	}
}

func TestAPIRejections(t *testing.T) {
	tests := []struct {
		name     string
		response clitest.Response
		args     []string
		exit     exit.Code
		code     string
	}{
		{
			name:     "get unknown job",
			response: clitest.Failure{Status: 404, Code: "job_not_found", Message: "Job not found"}.Response(),
			args:     []string{"get", jobID},
			exit:     exit.NotFound,
			code:     "job_not_found",
		},
		{
			name:     "repeated idempotency key",
			response: clitest.Failure{Status: 409, Code: "idempotency_key_conflict", Message: "A job with this idempotency key already exists"}.Response(),
			args:     []string{"create", "--name", "Report", "--handler", "report", "--idempotency-key", "k1"},
			exit:     exit.Rejected,
			code:     "idempotency_key_conflict",
		},
		{
			name:     "retry a job that did not fail",
			response: clitest.Failure{Status: 409, Code: "job_not_failed", Message: "Only failed jobs can be retried"}.Response(),
			args:     []string{"retry", jobID},
			exit:     exit.Rejected,
			code:     "job_not_failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := clitest.NewAPI(t, tt.response)
			res := run(t, api, append([]string{"--json", "jobs"}, tt.args...)...)
			clitest.WantExit(t, res, tt.exit)
			if code := clitest.DecodeError(t, res.Stderr).Code; code != tt.code {
				t.Errorf("stderr = %s, want code %s", res.Stderr, tt.code)
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("requests = %d, want 1", n)
			}
		})
	}
}

func TestNextPageHintRepeatsTheResolvedTimeWindow(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Page("["+failedJob(t)+"]", chronos.Paging{Page: 1, Pages: 2, Size: 1, Total: 2}))
	res := clitest.Run(t, clitest.Options{
		Args:      []string{"jobs", "list", "--scheduled-after", "1h", "--status", "failed"},
		Env:       noColor(api.Env()),
		StdoutTTY: true,
	})
	clitest.WantExit(t, res, exit.OK)
	want := "Next page: chronos jobs list --scheduled-after 2026-10-28T11:00:00Z --status failed --page 2"
	if !strings.Contains(res.Stderr, want) {
		t.Errorf("stderr = %q, want %q", res.Stderr, want)
	}
}

func TestPushURLPasswordIsRedacted(t *testing.T) {
	const pushURL = "https://user:secret@example.com/hooks/report?sig=abc"
	t.Run("terminal", func(t *testing.T) {
		pushed := job(t, map[string]any{"delivery": map[string]any{"type": "push", "http": map[string]any{"url": pushURL, "method": "POST"}}})
		api := clitest.NewAPI(t, clitest.OK(pushed))
		res := clitest.Run(t, clitest.Options{Args: []string{"jobs", "get", jobID}, Env: noColor(api.Env()), StdoutTTY: true})
		clitest.WantExit(t, res, exit.OK)
		if !strings.Contains(res.Stdout, "POST https://user:xxxxx@example.com/hooks/report?sig=abc") || strings.Contains(res.Stdout, "secret") {
			t.Errorf("stdout = %s, want the push URL with its password hidden", res.Stdout)
		}
	})
	t.Run("dry run", func(t *testing.T) {
		api := clitest.NewAPI(t)
		res := clitest.Run(t, clitest.Options{
			Args: []string{"--json", "jobs", "create", "--name", "Report", "--push-url", pushURL, "--dry-run"},
			Env:  map[string]string{"CHRONOS_BASE_URL": api.URL},
		})
		clitest.WantExit(t, res, exit.OK)
		if !strings.Contains(res.Stdout, `"url":"https://user:xxxxx@example.com/hooks/report?sig=abc"`) || strings.Contains(res.Stdout, "secret") {
			t.Errorf("stdout = %s, want the push URL with its password hidden", res.Stdout)
		}
	})
}
