package monitors_test

import (
	"strings"
	"testing"

	chronos "chronos.sh/go"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/exit"
)

func TestList(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Page("["+monitor(t, nil)+"]", chronos.Paging{Page: 2, Pages: 3, Size: 1, Total: 3}))
	res := run(t, api, "--json", "monitors", "list", "--status", "down", "--search", "backup", "--page", "2", "--size", "1")
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "list", transcript(api, res))
}

func TestListAll(t *testing.T) {
	interval := monitor(t, map[string]any{
		"id": "0192f3a0-0000-7000-8000-000000000019", "cron": nil, "interval": map[string]any{"value": 1, "unit": "hour"},
	})
	api := clitest.NewAPI(t,
		clitest.Page("["+monitor(t, nil)+"]", chronos.Paging{Page: 1, Pages: 2, Size: 1, Total: 2}),
		clitest.Page("["+interval+"]", chronos.Paging{Page: 2, Pages: 2, Size: 1, Total: 2}),
	)
	res := run(t, api, "--json", "monitors", "list", "--all", "--size", "1")
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "list_all", transcript(api, res))
}

func TestListPageAndAllConflict(t *testing.T) {
	for _, args := range [][]string{{"list"}, {"list-pings", monitorID}} {
		t.Run(args[0], func(t *testing.T) {
			api := clitest.NewAPI(t)
			res := run(t, api, append(append([]string{"--json", "monitors"}, args...), "--all", "--page", "2")...)
			clitest.WantExit(t, res, exit.Usage)
			wantNoRequests(t, api)
		})
	}
}

func TestListInATerminal(t *testing.T) {
	fresh := newMonitor(t, map[string]any{
		"id": "0192f3a0-0000-7000-8000-000000000019", "name": "Sync", "cron": nil,
		"interval": map[string]any{"value": 1, "unit": "hour"},
	})
	api := clitest.NewAPI(t, clitest.Page("["+monitor(t, nil)+","+fresh+"]", chronos.Paging{Page: 1, Pages: 2, Size: 2, Total: 4}))
	env := api.Env()
	env["NO_COLOR"] = "1"
	res := clitest.Run(t, clitest.Options{Args: []string{"monitors", "list", "--size", "2"}, Env: env, StdoutTTY: true, Width: 140})
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "list_terminal", transcript(api, res))
}

func TestGet(t *testing.T) {
	api := clitest.NewAPI(t, clitest.OK(monitor(t, nil)))
	res := run(t, api, "--json", "monitors", "get", monitorID)
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "get", transcript(api, res))
}

func TestGetInATerminal(t *testing.T) {
	failed := monitor(t, map[string]any{
		"description": "Backs up the database", "status": "down",
		"last_ping": map[string]any{"kind": "fail", "exit_code": 2, "received_at": "2026-10-24T03:01:12.000Z"},
	})
	api := clitest.NewAPI(t, clitest.OK(failed))
	res := clitest.Run(t, clitest.Options{Args: []string{"monitors", "get", monitorID}, Env: api.Env(), StdoutTTY: true})
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "get_terminal", transcript(api, res))
}

func TestGetMarksAPingURLTheKeyCannotSee(t *testing.T) {
	api := clitest.NewAPI(t, clitest.OK(monitor(t, map[string]any{"ping_url": "https://ping.chronos.sh/****"})))
	res := clitest.Run(t, clitest.Options{Args: []string{"monitors", "get", monitorID}, Env: api.Env(), StdoutTTY: true})
	clitest.WantExit(t, res, exit.OK)
	if !strings.Contains(res.Stdout, "https://ping.chronos.sh/**** (token hidden because your key lacks the monitors:write scope)") {
		t.Errorf("stdout = %s", res.Stdout)
	}
}

func TestGetNotFound(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Failure{Status: 404, Code: "monitor_not_found", Message: "Monitor not found"}.Response())
	res := run(t, api, "--json", "monitors", "get", monitorID)
	clitest.WantExit(t, res, exit.NotFound)
	if code := clitest.DecodeError(t, res.Stderr).Code; code != "monitor_not_found" {
		t.Errorf("error code = %q", code)
	}
}

func TestListPings(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Page("["+failedPing(t)+","+ping(t, nil)+"]", chronos.Paging{Page: 1, Pages: 1, Size: 5, Total: 2}))
	res := run(t, api, "--json", "monitors", "list-pings", monitorID, "--size", "5")
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "list_pings", transcript(api, res))
}

func TestListPingsAll(t *testing.T) {
	api := clitest.NewAPI(t,
		clitest.Page("["+ping(t, nil)+"]", chronos.Paging{Page: 1, Pages: 2, Size: 1, Total: 2}),
		clitest.Page("["+ping(t, map[string]any{"id": "0192f3a0-0000-7000-8000-000000000023", "kind": "start", "exit_code": nil, "duration_ms": nil})+"]",
			chronos.Paging{Page: 2, Pages: 2, Size: 1, Total: 2}),
	)
	res := run(t, api, "--json", "monitors", "list-pings", monitorID, "--all", "--size", "1")
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "list_pings_all", transcript(api, res))
}

func TestListPingsInATerminal(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Page("["+failedPing(t)+","+ping(t, nil)+"]", chronos.Paging{Page: 1, Pages: 1, Size: 20, Total: 2}))
	env := api.Env()
	env["NO_COLOR"] = "1"
	res := clitest.Run(t, clitest.Options{Args: []string{"monitors", "list-pings", monitorID}, Env: env, StdoutTTY: true, Width: 120})
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "list_pings_terminal", transcript(api, res))
}

func TestListPingsEscapesTerminalSequences(t *testing.T) {
	hostile := ping(t, map[string]any{
		"body":       "\x1b]52;c;cm0gLXJmIH4=\x07copied\u009b2J\u202e",
		"user_agent": "\x1b]8;;https://evil.example\x1b\\click\x1b]8;;\x1b\\",
	})
	for _, tty := range []bool{true, false} {
		api := clitest.NewAPI(t, clitest.Page("["+hostile+"]", chronos.Paging{Page: 1, Pages: 1, Size: 20, Total: 1}))
		res := clitest.Run(t, clitest.Options{Args: []string{"monitors", "list-pings", monitorID}, Env: api.Env(), StdoutTTY: tty, Width: 400})
		clitest.WantExit(t, res, exit.OK)
		if strings.ContainsAny(res.Stdout, "\x1b\x07\u009b\u202e") {
			t.Errorf("tty=%v: stdout carries a raw control character: %q", tty, res.Stdout)
		}
		if !strings.Contains(res.Stdout, `\x1b]52;c;`) {
			t.Errorf("tty=%v: stdout = %q, want the escape shown as text", tty, res.Stdout)
		}
	}
}
