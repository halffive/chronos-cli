package schedules_test

import (
	"testing"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/exit"
)

func TestDeleteWithYes(t *testing.T) {
	for _, flag := range []string{"--yes", "-y"} {
		t.Run(flag, func(t *testing.T) {
			api := clitest.NewAPI(t, clitest.Response{Status: 204})
			res := run(t, api, "--json", "schedules", "delete", scheduleID, flag)
			wantExit(t, res, exit.OK)
			clitest.Golden(t, "delete", transcript(api, res))
		})
	}
}

func TestDeleteNeedsYesWithoutAPrompt(t *testing.T) {
	tests := []struct {
		name string
		tty  bool
		args []string
	}{
		{"no terminal", false, []string{"schedules", "delete", scheduleID}},
		{"json in a terminal", true, []string{"--json", "schedules", "delete", scheduleID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := clitest.NewAPI(t)
			res := clitest.Run(t, clitest.Options{Args: tt.args, Env: api.Env(), StdinTTY: tt.tty})
			wantExit(t, res, exit.Usage)
			wantNoRequests(t, api)
		})
		t.Run(tt.name+" without a key", func(t *testing.T) {
			res := clitest.Run(t, clitest.Options{Args: tt.args, StdinTTY: tt.tty})
			wantExit(t, res, exit.Usage)
		})
	}
}

func TestDeletePrompt(t *testing.T) {
	tests := []struct {
		name   string
		answer string
		exit   exit.Code
	}{
		{"delete_confirmed", "y\n", exit.OK},
		{"delete_declined", "n\n", exit.Failure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := clitest.NewAPI(t, clitest.OK(schedule(t, nil)), clitest.Response{Status: 204})
			res := clitest.Run(t, clitest.Options{
				Args:     []string{"schedules", "delete", scheduleID},
				Env:      api.Env(),
				StdinTTY: true,
				Stdin:    tt.answer,
			})
			wantExit(t, res, tt.exit)
			clitest.Golden(t, tt.name, transcript(api, res))
		})
	}
}

func TestDeletePromptForAMissingSchedule(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Failure{Status: 404, Code: "schedule_not_found", Message: "Schedule not found"}.Response())
	res := clitest.Run(t, clitest.Options{Args: []string{"schedules", "delete", scheduleID}, Env: api.Env(), StdinTTY: true, Stdin: "y\n"})
	wantExit(t, res, exit.NotFound)
	if n := len(api.Requests()); n != 1 {
		t.Errorf("requests = %d, want only the lookup", n)
	}
}
