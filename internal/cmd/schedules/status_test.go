package schedules_test

import (
	"strings"
	"testing"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/exit"
)

func TestPauseAndResume(t *testing.T) {
	tests := []struct {
		verb   string
		status string
	}{
		{"pause", "paused"},
		{"resume", "active"},
	}
	for _, tt := range tests {
		t.Run(tt.verb, func(t *testing.T) {
			api := clitest.NewAPI(t, clitest.OK(schedule(t, map[string]any{"status": tt.status})))
			res := run(t, api, "--json", "schedules", tt.verb, scheduleID)
			clitest.WantExit(t, res, exit.OK)
			clitest.Golden(t, tt.verb, transcript(api, res))
		})
	}
}

func TestPauseAndResumeWhenAlreadyThere(t *testing.T) {
	tests := []struct {
		verb   string
		code   string
		status string
	}{
		{"pause", "schedule_already_paused", "paused"},
		{"resume", "schedule_already_active", "active"},
	}
	for _, tt := range tests {
		t.Run(tt.verb, func(t *testing.T) {
			api := clitest.NewAPI(t,
				clitest.Failure{Status: 409, Code: tt.code, Message: "Schedule is already " + tt.status}.Response(),
				clitest.OK(schedule(t, map[string]any{"status": tt.status})),
			)
			res := run(t, api, "schedules", tt.verb, scheduleID)
			clitest.WantExit(t, res, exit.OK)
			clitest.Golden(t, tt.verb+"_already", transcript(api, res))
		})
	}
}

func TestPauseAndResumeAlreadyThereWithJSONPrintsOnlyTheSchedule(t *testing.T) {
	api := clitest.NewAPI(t,
		clitest.Failure{Status: 409, Code: "schedule_already_paused", Message: "Schedule is already paused"}.Response(),
		clitest.OK(schedule(t, map[string]any{"status": "paused"})),
	)
	res := run(t, api, "--json", "schedules", "pause", scheduleID)
	clitest.WantExit(t, res, exit.OK)
	if res.Stderr != "" || !strings.Contains(res.Stdout, `"status":"paused"`) {
		t.Errorf("stdout = %s\nstderr = %q", res.Stdout, res.Stderr)
	}
}

func TestPauseAndResumeRejectOtherConflicts(t *testing.T) {
	tests := []struct {
		verb string
		code string
	}{
		{"pause", "schedule_archived"},
		{"pause", "schedule_already_active"},
		{"resume", "schedule_completed"},
	}
	for _, tt := range tests {
		t.Run(tt.verb+" "+tt.code, func(t *testing.T) {
			api := clitest.NewAPI(t, clitest.Failure{Status: 409, Code: tt.code, Message: "Conflict"}.Response())
			res := run(t, api, "--json", "schedules", tt.verb, scheduleID)
			clitest.WantExit(t, res, exit.Rejected)
			if code := clitest.DecodeError(t, res.Stderr).Code; code != tt.code {
				t.Errorf("error code = %q, want %q", code, tt.code)
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("requests = %d, want 1", n)
			}
		})
	}
}

func TestPauseAndResumeAlreadyThereButInAnotherStatus(t *testing.T) {
	api := clitest.NewAPI(t,
		clitest.Failure{Status: 409, Code: "schedule_already_paused", Message: "Schedule is already paused"}.Response(),
		clitest.OK(schedule(t, map[string]any{"status": "archived"})),
	)
	res := run(t, api, "--json", "schedules", "pause", scheduleID)
	clitest.WantExit(t, res, exit.Rejected)
	if code := clitest.DecodeError(t, res.Stderr).Code; code != "schedule_already_paused" {
		t.Errorf("error code = %q", code)
	}
}

func TestPauseAndResumeAlreadyThereButTheLookupFails(t *testing.T) {
	api := clitest.NewAPI(t,
		clitest.Failure{Status: 409, Code: "schedule_already_paused", Message: "Schedule is already paused"}.Response(),
		clitest.Failure{Status: 404, Code: "schedule_not_found", Message: "Schedule not found"}.Response(),
	)
	res := run(t, api, "--json", "schedules", "pause", scheduleID)
	clitest.WantExit(t, res, exit.NotFound)
}

func TestArchive(t *testing.T) {
	api := clitest.NewAPI(t, clitest.OK(schedule(t, map[string]any{"status": "archived", "next_run_at": nil})))
	res := run(t, api, "--json", "schedules", "archive", scheduleID)
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "archive", transcript(api, res))
}
