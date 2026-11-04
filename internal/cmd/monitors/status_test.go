package monitors_test

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
		{"resume", "new"},
	}
	responses := map[string]string{"paused": pausedMonitor(t), "new": newMonitor(t, nil)}
	for _, tt := range tests {
		t.Run(tt.verb, func(t *testing.T) {
			api := clitest.NewAPI(t, clitest.OK(responses[tt.status]))
			res := run(t, api, "--json", "monitors", tt.verb, monitorID)
			clitest.WantExit(t, res, exit.OK)
			clitest.Golden(t, tt.verb, transcript(api, res))
		})
	}
}

func TestPauseWhenAlreadyPaused(t *testing.T) {
	api := clitest.NewAPI(t,
		clitest.Failure{Status: 409, Code: "monitor_already_paused", Message: "Monitor is already paused"}.Response(),
		clitest.OK(pausedMonitor(t)),
	)
	res := run(t, api, "monitors", "pause", monitorID)
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "pause_already", transcript(api, res))
}

func TestResumeWhenNotPaused(t *testing.T) {
	for _, status := range []string{"new", "up", "late", "down"} {
		t.Run(status, func(t *testing.T) {
			api := clitest.NewAPI(t,
				clitest.Failure{Status: 409, Code: "monitor_not_paused", Message: "Monitor is not paused"}.Response(),
				clitest.OK(monitor(t, map[string]any{"status": status})),
			)
			res := run(t, api, "monitors", "resume", monitorID)
			clitest.WantExit(t, res, exit.OK)
			want := "Monitor " + monitorID + " is not paused, so nothing changed.\n"
			if res.Stderr != want {
				t.Errorf("stderr = %q, want %q", res.Stderr, want)
			}
		})
	}
}

func TestPauseAlreadyPausedWithJSONPrintsOnlyTheMonitor(t *testing.T) {
	api := clitest.NewAPI(t,
		clitest.Failure{Status: 409, Code: "monitor_already_paused", Message: "Monitor is already paused"}.Response(),
		clitest.OK(pausedMonitor(t)),
	)
	res := run(t, api, "--json", "monitors", "pause", monitorID)
	clitest.WantExit(t, res, exit.OK)
	if res.Stderr != "" || !strings.Contains(res.Stdout, `"status":"paused"`) {
		t.Errorf("stdout = %s\nstderr = %q", res.Stdout, res.Stderr)
	}
}

func TestPauseAndResumeConflictThatTheLookupContradicts(t *testing.T) {
	tests := []struct {
		verb   string
		code   string
		status string
	}{
		{"pause", "monitor_already_paused", "up"},
		{"resume", "monitor_not_paused", "paused"},
	}
	for _, tt := range tests {
		t.Run(tt.verb, func(t *testing.T) {
			api := clitest.NewAPI(t,
				clitest.Failure{Status: 409, Code: tt.code, Message: "Conflict"}.Response(),
				clitest.OK(monitor(t, map[string]any{"status": tt.status})),
			)
			res := run(t, api, "--json", "monitors", tt.verb, monitorID)
			clitest.WantExit(t, res, exit.Rejected)
			if code := clitest.DecodeError(t, res.Stderr).Code; code != tt.code {
				t.Errorf("error code = %q, want %q", code, tt.code)
			}
		})
	}
}

func TestPauseAndResumeConflictButTheLookupFails(t *testing.T) {
	api := clitest.NewAPI(t,
		clitest.Failure{Status: 409, Code: "monitor_already_paused", Message: "Monitor is already paused"}.Response(),
		clitest.Failure{Status: 404, Code: "monitor_not_found", Message: "Monitor not found"}.Response(),
	)
	res := run(t, api, "--json", "monitors", "pause", monitorID)
	clitest.WantExit(t, res, exit.NotFound)
}

func TestPauseHidesThePingURLToken(t *testing.T) {
	api := clitest.NewAPI(t, clitest.OK(pausedMonitor(t)))
	res := run(t, api, "monitors", "pause", monitorID)
	clitest.WantExit(t, res, exit.OK)
	if strings.Contains(res.Stdout, "k3Jx9Qm2TnV7bRw4YcZ8dHfL") {
		t.Errorf("stdout = %s, want the ping URL token hidden", res.Stdout)
	}
}

func TestPauseAndResumeRejectOtherErrors(t *testing.T) {
	tests := []struct {
		verb   string
		status int
		code   string
	}{
		{"pause", 409, "monitor_not_paused"},
		{"resume", 409, "monitor_already_paused"},
		{"resume", 403, "plan_limit_exceeded"},
	}
	for _, tt := range tests {
		t.Run(tt.verb+" "+tt.code, func(t *testing.T) {
			api := clitest.NewAPI(t, clitest.Failure{Status: tt.status, Code: tt.code, Message: "Rejected"}.Response())
			res := run(t, api, "--json", "monitors", tt.verb, monitorID)
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

func TestRotatePingURL(t *testing.T) {
	api := clitest.NewAPI(t, clitest.OK(monitor(t, map[string]any{"ping_url": "https://ping.chronos.sh/Zq8vN2kWm4XtR7bJc9LdHf3P"})))
	res := run(t, api, "--json", "monitors", "rotate-ping-url", monitorID)
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "rotate_ping_url", transcript(api, res))
}
