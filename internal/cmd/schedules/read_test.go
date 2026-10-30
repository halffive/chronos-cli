package schedules_test

import (
	"testing"

	chronos "chronos.sh/go"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/exit"
)

func TestList(t *testing.T) {
	list := "[" + schedule(t, nil) + "]"
	api := clitest.NewAPI(t, clitest.Page(list, chronos.Paging{Page: 2, Pages: 3, Size: 1, Total: 3}))
	res := run(t, api, "--json", "schedules", "list", "--status", "active", "--search", "nightly", "--page", "2", "--size", "1")
	wantExit(t, res, exit.OK)
	clitest.Golden(t, "list", transcript(api, res))
}

func TestListAll(t *testing.T) {
	interval := schedule(t, map[string]any{
		"id": "0192f3a0-0000-7000-8000-000000000009", "cron": nil, "interval": map[string]any{"value": 5, "unit": "minute"},
	})
	api := clitest.NewAPI(t,
		clitest.Page("["+schedule(t, nil)+"]", chronos.Paging{Page: 1, Pages: 2, Size: 1, Total: 2}),
		clitest.Page("["+interval+"]", chronos.Paging{Page: 2, Pages: 2, Size: 1, Total: 2}),
	)
	res := run(t, api, "--json", "schedules", "list", "--all", "--size", "1")
	wantExit(t, res, exit.OK)
	clitest.Golden(t, "list_all", transcript(api, res))
}

func TestListPageAndAllConflict(t *testing.T) {
	api := clitest.NewAPI(t)
	res := run(t, api, "--json", "schedules", "list", "--all", "--page", "2")
	wantExit(t, res, exit.Usage)
	wantNoRequests(t, api)
}

func TestListInATerminal(t *testing.T) {
	interval := schedule(t, map[string]any{
		"id": "0192f3a0-0000-7000-8000-000000000009", "name": "Sync", "cron": nil,
		"interval": map[string]any{"value": 5, "unit": "minute"}, "status": "paused", "next_run_at": nil,
		"last_run_at": "2026-10-28T11:55:00.000Z", "delivery": map[string]any{"type": "pull"},
	})
	api := clitest.NewAPI(t, clitest.Page("["+schedule(t, nil)+","+interval+"]", chronos.Paging{Page: 1, Pages: 2, Size: 2, Total: 4}))
	env := api.Env()
	env["NO_COLOR"] = "1"
	res := clitest.Run(t, clitest.Options{Args: []string{"schedules", "list", "--size", "2"}, Env: env, StdoutTTY: true, Width: 120})
	wantExit(t, res, exit.OK)
	clitest.Golden(t, "list_terminal", transcript(api, res))
}

func TestGet(t *testing.T) {
	api := clitest.NewAPI(t, clitest.OK(schedule(t, nil)))
	res := run(t, api, "--json", "schedules", "get", scheduleID)
	wantExit(t, res, exit.OK)
	clitest.Golden(t, "get", transcript(api, res))
}

func TestGetInATerminal(t *testing.T) {
	pushed := schedule(t, map[string]any{
		"description": "Sends the daily report",
		"runs":        10,
		"run_count":   2,
		"delivery": map[string]any{"type": "push", "http": map[string]any{
			"url": "https://example.com/hooks/report", "method": "POST",
			"headers": map[string]any{"Authorization": "Bearer downstream-secret", "X-Team": "ops"},
		}},
	})
	api := clitest.NewAPI(t, clitest.OK(pushed))
	res := clitest.Run(t, clitest.Options{Args: []string{"schedules", "get", scheduleID}, Env: api.Env(), StdoutTTY: true})
	wantExit(t, res, exit.OK)
	clitest.Golden(t, "get_terminal", transcript(api, res))
}

func TestGetNotFound(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Failure{Status: 404, Code: "schedule_not_found", Message: "Schedule not found"}.Response())
	res := run(t, api, "--json", "schedules", "get", scheduleID)
	wantExit(t, res, exit.NotFound)
	if code := jsonError(t, res.Stderr).Code; code != "schedule_not_found" {
		t.Errorf("error code = %q", code)
	}
}
