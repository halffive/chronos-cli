package schedules_test

import (
	"strings"
	"testing"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/exit"
)

func TestDryRunSendsNothing(t *testing.T) {
	tests := [][]string{
		{"create", "--name", "Sync", "--interval", "5m", "--push-url", "https://example.com/hooks/sync", "--push-header", "Authorization: Bearer downstream-secret"},
		{"update", scheduleID, "--cron", "0 4 * * *", "--clear", "runs"},
		{"pause", scheduleID},
		{"resume", scheduleID},
		{"trigger", scheduleID},
		{"trigger", scheduleID, "--wait"},
		{"archive", scheduleID},
		{"delete", scheduleID},
	}
	var golden strings.Builder
	for _, args := range tests {
		t.Run(strings.Join(args[:1], " "), func(t *testing.T) {
			api := clitest.NewAPI(t)
			res := clitest.Run(t, clitest.Options{
				Args:     append([]string{"--json", "schedules"}, append(args, "--dry-run")...),
				Env:      api.Env(),
				StdinTTY: true,
			})
			clitest.WantExit(t, res, exit.OK)
			wantNoRequests(t, api)
			golden.WriteString("$ chronos schedules " + strings.Join(args, " ") + " --dry-run\n")
			golden.WriteString(strings.ReplaceAll(res.Stdout, api.URL, "http://api.test"))
		})
	}
	clitest.Golden(t, "dry_run", golden.String())
}

func TestDeleteDryRunSkipsThePromptLookup(t *testing.T) {
	api := clitest.NewAPI(t)
	res := clitest.Run(t, clitest.Options{Args: []string{"schedules", "delete", scheduleID, "--dry-run"}, Env: api.Env(), StdinTTY: true})
	clitest.WantExit(t, res, exit.OK)
	wantNoRequests(t, api)
	if !strings.Contains(res.Stdout, "DELETE http://") {
		t.Errorf("stdout = %s", res.Stdout)
	}
}
