package monitors_test

import (
	"strings"
	"testing"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/exit"
)

func TestDryRunSendsNothing(t *testing.T) {
	tests := [][]string{
		{"create", "--name", "Sync", "--interval", "1h", "--grace", "10m"},
		{"update", monitorID, "--cron", "0 4 * * *", "--clear", "description"},
		{"pause", monitorID},
		{"resume", monitorID},
		{"rotate-ping-url", monitorID},
		{"delete", monitorID},
	}
	var golden strings.Builder
	for _, args := range tests {
		t.Run(args[0], func(t *testing.T) {
			api := clitest.NewAPI(t)
			res := clitest.Run(t, clitest.Options{
				Args:     append([]string{"--json", "monitors"}, append(args, "--dry-run")...),
				Env:      api.Env(),
				StdinTTY: true,
			})
			clitest.WantExit(t, res, exit.OK)
			wantNoRequests(t, api)
			golden.WriteString("$ chronos monitors " + strings.Join(args, " ") + " --dry-run\n")
			golden.WriteString(strings.ReplaceAll(res.Stdout, api.URL, "http://api.test"))
		})
	}
	clitest.Golden(t, "dry_run", golden.String())
}

func TestDeleteDryRunSkipsThePromptLookup(t *testing.T) {
	api := clitest.NewAPI(t)
	res := clitest.Run(t, clitest.Options{Args: []string{"monitors", "delete", monitorID, "--dry-run"}, Env: api.Env(), StdinTTY: true})
	clitest.WantExit(t, res, exit.OK)
	wantNoRequests(t, api)
	if !strings.Contains(res.Stdout, "DELETE http://") {
		t.Errorf("stdout = %s", res.Stdout)
	}
}
