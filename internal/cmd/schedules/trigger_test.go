package schedules_test

import (
	"strings"
	"testing"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/exit"
)

func TestTrigger(t *testing.T) {
	api := clitest.NewAPI(t, created(job(t, "queued")))
	res := run(t, api, "--json", "schedules", "trigger", scheduleID)
	wantExit(t, res, exit.OK)
	clitest.Golden(t, "trigger", transcript(api, res))
}

func TestTriggerInATerminal(t *testing.T) {
	api := clitest.NewAPI(t, created(job(t, "queued")))
	res := clitest.Run(t, clitest.Options{Args: []string{"schedules", "trigger", scheduleID}, Env: api.Env(), StdoutTTY: true})
	wantExit(t, res, exit.OK)
	clitest.Golden(t, "trigger_terminal", transcript(api, res))
}

func TestTriggerWait(t *testing.T) {
	tests := []struct {
		name      string
		responses []clitest.Response
		args      []string
		exit      exit.Code
		code      string
	}{
		{
			name:      "trigger_wait_completed",
			responses: []clitest.Response{created(job(t, "queued")), clitest.OK(job(t, "running")), clitest.OK(job(t, "completed"))},
			exit:      exit.OK,
		},
		{
			name:      "trigger_wait_failed",
			responses: []clitest.Response{created(job(t, "queued")), clitest.OK(job(t, "failed"))},
			exit:      exit.JobUnsuccessful,
			code:      exit.CodeJobUnsuccessful,
		},
		{
			name:      "trigger_wait_timeout",
			responses: []clitest.Response{created(job(t, "queued")), clitest.OK(job(t, "running"))},
			args:      []string{"--wait-timeout", "1"},
			exit:      exit.WaitTimeout,
			code:      exit.CodeWaitTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := clitest.NewAPI(t, tt.responses...)
			res := run(t, api, append([]string{"--json", "schedules", "trigger", scheduleID, "--wait"}, tt.args...)...)
			wantExit(t, res, tt.exit)
			if tt.code == "" {
				clitest.Golden(t, tt.name, transcript(api, res))
				return
			}
			if e := jsonError(t, res.Stderr); e.Code != tt.code || e.Retryable {
				t.Errorf("error = %+v, want code %s and not retryable, since the trigger was applied", e, tt.code)
			}
			// The polling count varies, so the golden file holds the trigger
			// request and what was printed.
			trigger := api.Requests()[0]
			clitest.Golden(t, tt.name, trigger.Method+" "+trigger.RequestURI+"\n--- stdout\n"+res.Stdout)
		})
	}
}

func TestTriggerWaitTimeoutIsCheckedBeforeTheTrigger(t *testing.T) {
	for _, timeout := range []string{"0", "1.5m", "soon"} {
		t.Run(timeout, func(t *testing.T) {
			api := clitest.NewAPI(t)
			res := run(t, api, "--json", "schedules", "trigger", scheduleID, "--wait", "--wait-timeout", timeout)
			wantExit(t, res, exit.Usage)
			wantNoRequests(t, api)
			if !strings.Contains(res.Stderr, "usage_error") {
				t.Errorf("stderr = %s", res.Stderr)
			}
		})
	}
}

func TestTriggerWaitTimeoutNeedsWait(t *testing.T) {
	api := clitest.NewAPI(t)
	res := run(t, api, "--json", "schedules", "trigger", scheduleID, "--wait-timeout", "90s")
	wantExit(t, res, exit.Usage)
	wantNoRequests(t, api)
}
