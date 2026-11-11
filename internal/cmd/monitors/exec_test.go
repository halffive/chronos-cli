package monitors_test

import (
	"context"
	"net/http"
	"os"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/cmd/monitors"
	"chronos.sh/cli/internal/cmd/root"
	"chronos.sh/cli/internal/exit"
)

const systemFlag = "-chronos-test-system"

func TestMain(m *testing.M) {
	clitest.ServeChild()
	if len(os.Args) > 1 && os.Args[1] == systemFlag {
		os.Exit(int(root.Run(context.Background(), app.NewSystemFactory(), os.Args[2:])))
	}
	os.Exit(m.Run())
}

const pingToken = "k3Jx9Qm2TnV7bRw4YcZ8dHfL"

func pingServer(t *testing.T, responses ...clitest.Response) (*clitest.API, string) {
	api := clitest.NewAPI(t, responses...)
	return api, api.URL + "/" + pingToken
}

func pinged() clitest.Response { return clitest.Response{Status: http.StatusOK} }

type execRun struct {
	flags []string
	steps []string
	env   map[string]string
	tty   bool
}

func runExec(t *testing.T, r execRun) clitest.Result {
	t.Helper()
	args := append(append([]string{"monitors", "exec"}, r.flags...), "--")
	args = append(args, clitest.ChildCommand(r.steps...)...)
	res := clitest.Run(t, clitest.Options{Args: args, Env: r.env, StderrTTY: r.tty, PingTimeout: 500 * time.Millisecond})
	if strings.Contains(res.Stdout+res.Stderr, pingToken) {
		t.Errorf("the output shows the ping token:\n%s", res.Stderr)
	}
	return res
}

func paths(api *clitest.API) []string {
	var got []string
	for _, r := range api.Requests() {
		got = append(got, strings.TrimPrefix(r.RequestURI, "/"+pingToken))
	}
	return got
}

func TestExecPingsStartThenTheExitCode(t *testing.T) {
	api, url := pingServer(t, pinged())
	res := runExec(t, execRun{flags: []string{"--ping-url", url}, steps: []string{"sleep:300ms", "out:dumping database app", "err:backup failed", "exit:3"}})
	clitest.WantExit(t, res, 3)
	if res.Stdout != "dumping database app\n" || res.Stderr != "backup failed\n" {
		t.Errorf("stdout = %q, stderr = %q; want the command's own output", res.Stdout, res.Stderr)
	}
	if got := paths(api); !slices.Equal(got, []string{"/start", "/3"}) {
		t.Fatalf("pings = %q, want /start then /3", got)
	}
	last := api.Requests()[1]
	if !strings.Contains(last.Body, "dumping database app\n") || !strings.Contains(last.Body, "backup failed\n") {
		t.Errorf("body = %q, want both streams", last.Body)
	}
	if ua := last.Header.Get("User-Agent"); !strings.HasPrefix(ua, "chronos-cli/") {
		t.Errorf("User-Agent = %q", ua)
	}
}

func TestExecKeepsTheExitCodeWhenPingsFail(t *testing.T) {
	t.Run("start ping answered 404", func(t *testing.T) {
		api, url := pingServer(t, clitest.Response{Status: http.StatusNotFound})
		res := runExec(t, execRun{flags: []string{"--ping-url", url}, steps: []string{"sleep:300ms", "exit:3"}})
		clitest.WantExit(t, res, 3)
		if got := paths(api); !slices.Equal(got, []string{"/start"}) {
			t.Errorf("pings = %q, want only /start", got)
		}
		want := "chronos: " + api.URL + "/•••• answered 404 to the start ping, so this run sends no more pings. The URL is wrong or was rotated. Run chronos monitors get <id> for the current one.\n"
		if res.Stderr != want {
			t.Errorf("stderr = %q, want %q", res.Stderr, want)
		}
	})
	t.Run("ping host never answers", func(t *testing.T) {
		api, url := pingServer(t, clitest.Response{Status: http.StatusOK, Delay: 10 * time.Second})
		started := time.Now()
		res := runExec(t, execRun{flags: []string{"--ping-url", url}, steps: []string{"exit:0"}})
		clitest.WantExit(t, res, exit.OK)
		if elapsed := time.Since(started); elapsed > 3*time.Second {
			t.Errorf("chronos lingered %s after the command, want one ping's timeout", elapsed)
		}
		want := "chronos: could not send exit code 0 to " + api.URL + "/•••• (no response within 500ms), so the monitor did not get this run's result.\n"
		if !strings.HasSuffix(res.Stderr, want) {
			t.Errorf("stderr = %q, want it to end %q", res.Stderr, want)
		}
	})
}

func TestExecReportsACommandThatCannotStart(t *testing.T) {
	api, url := pingServer(t, pinged())
	res := clitest.Run(t, clitest.Options{Args: []string{"monitors", "exec", "--ping-url", url, "chronos-no-such-command"}})
	clitest.WantExit(t, res, 127)
	line := "chronos: chronos-no-such-command: executable file not found in $PATH"
	if res.Stderr != line+"\n" {
		t.Errorf("stderr = %q", res.Stderr)
	}
	if got := paths(api); !slices.Equal(got, []string{"/127"}) || api.Requests()[0].Body != line {
		t.Errorf("pings = %q with body %q, want only /127 with the error", got, api.Requests()[0].Body)
	}
}

func TestExecMasksAPingURLPassedAsTheCommand(t *testing.T) {
	api, url := pingServer(t, pinged())
	res := clitest.Run(t, clitest.Options{Args: []string{"monitors", "exec", "https://ping.chronos.sh/" + pingToken}, Env: map[string]string{"CHRONOS_PING_URL": url}})
	clitest.WantExit(t, res, 127)
	if !strings.HasPrefix(res.Stderr, "chronos: https://ping.chronos.sh/••••: ") || strings.Contains(res.Stderr, pingToken) {
		t.Errorf("stderr = %q, want the URL masked", res.Stderr)
	}
	if body := api.Requests()[0].Body; strings.Contains(body, pingToken) {
		t.Errorf("the /127 body shows the token: %q", body)
	}
}

func TestExecSendsTheTailOfLongOutput(t *testing.T) {
	api, url := pingServer(t, pinged())
	res := runExec(t, execRun{flags: []string{"--ping-url", url}, steps: []string{"bytes:30000"}})
	clitest.WantExit(t, res, exit.OK)
	if len(res.Stdout) != 30000 {
		t.Errorf("stdout has %d bytes, want all 30000", len(res.Stdout))
	}
	body := api.Requests()[len(api.Requests())-1].Body
	if len(body) > 10240 || !strings.HasPrefix(body, "[chronos: dropped the first 19.4 KB of output]\n") {
		t.Errorf("body is %d bytes and starts %q", len(body), body[:min(len(body), 60)])
	}
}

func TestExecNoPingBody(t *testing.T) {
	api, url := pingServer(t, pinged())
	res := runExec(t, execRun{flags: []string{"--ping-url", url, "--no-ping-body"}, steps: []string{"out:secret=hunter2", "exit:1"}})
	clitest.WantExit(t, res, 1)
	if res.Stdout != "secret=hunter2\n" {
		t.Errorf("stdout = %q", res.Stdout)
	}
	for _, r := range api.Requests() {
		if r.Body != "" {
			t.Errorf("%s sent body %q", r.RequestURI, r.Body)
		}
	}
}

func TestExecRunsTheCommandWithoutAUsablePingURL(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		flag string
		want string
	}{
		{"none", nil, "", "chronos: no ping URL, so this run sends no pings. Set CHRONOS_PING_URL in the crontab or pass --ping-url."},
		{"blank", map[string]string{"CHRONOS_PING_URL": "  "}, "", "chronos: no ping URL, so this run sends no pings. Set CHRONOS_PING_URL in the crontab or pass --ping-url."},
		{"copied with /start", nil, "https://ping.chronos.sh/" + pingToken + "/start", "chronos: https://ping.chronos.sh/••••/start is not a ping URL, so this run sends no pings. Use the URL from chronos monitors get <id>, which ends with the token."},
		{"masked", nil, "https://ping.chronos.sh/****", "chronos: https://ping.chronos.sh/**** is a masked ping URL, so this run sends no pings. Read the full URL with chronos monitors get <id>, using a login or API key with monitors:write."},
		{"plain http", nil, "http://ping.chronos.sh/" + pingToken, "chronos: http://ping.chronos.sh/•••• must use https, so this run sends no pings. Plain http is allowed only for localhost."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var flags []string
			if tt.flag != "" {
				flags = []string{"--ping-url", tt.flag}
			}
			res := runExec(t, execRun{flags: flags, env: tt.env, steps: []string{"out:dumping database app", "exit:5"}})
			clitest.WantExit(t, res, 5)
			if res.Stdout != "dumping database app\n" || res.Stderr != tt.want+"\n" {
				t.Errorf("stdout = %q\nstderr = %q\nwant %q", res.Stdout, res.Stderr, tt.want)
			}
		})
	}
}

func TestExecPingURLComesFromTheFlagThenTheEnvironment(t *testing.T) {
	envAPI, envURL := pingServer(t, pinged())
	res := runExec(t, execRun{env: map[string]string{"CHRONOS_PING_URL": envURL}, steps: []string{"exit:0"}})
	clitest.WantExit(t, res, exit.OK)
	if len(envAPI.Requests()) == 0 {
		t.Error("CHRONOS_PING_URL got no pings")
	}

	unused, unusedURL := pingServer(t)
	flagAPI, flagURL := pingServer(t, pinged())
	res = runExec(t, execRun{flags: []string{"--ping-url", flagURL}, env: map[string]string{"CHRONOS_PING_URL": unusedURL}, steps: []string{"exit:0"}})
	clitest.WantExit(t, res, exit.OK)
	if len(flagAPI.Requests()) == 0 || len(unused.Requests()) != 0 {
		t.Errorf("--ping-url got %d pings and CHRONOS_PING_URL %d, want the flag to win", len(flagAPI.Requests()), len(unused.Requests()))
	}
}

func TestExecPassesFlagsAfterTheCommandToIt(t *testing.T) {
	_, url := pingServer(t, pinged())
	child := clitest.ChildCommand("-f", "--json", "--help")
	for _, dash := range [][]string{nil, {"--"}} {
		args := append(append([]string{"monitors", "exec", "--ping-url", url}, dash...), child...)
		res := clitest.Run(t, clitest.Options{Args: args})
		clitest.WantExit(t, res, exit.OK)
		if res.Stdout != "arg -f\narg --json\narg --help\n" || res.Stderr != "" {
			t.Errorf("%v: stdout = %q, stderr = %q", dash, res.Stdout, res.Stderr)
		}
	}
}

func TestExecClosingLineInATerminal(t *testing.T) {
	tests := []struct {
		name  string
		flags []string
		steps []string
		want  string
	}{
		{"output", nil, []string{"out:dumping database app"}, "sent exit code 0 and 21 bytes of output to"},
		{"no output", nil, []string{"exit:2"}, "sent exit code 2 and no output to"},
		{"long output", nil, []string{"bytes:30000"}, "sent exit code 0 and the last 10 KB of 29.3 KB of output to"},
		{"--no-ping-body", []string{"--no-ping-body"}, []string{"out:dumping database app"}, "sent exit code 0 to"},
		{"--json", []string{"--json"}, []string{"out:dumping database app"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, url := pingServer(t, pinged())
			res := runExec(t, execRun{flags: append(tt.flags, "--ping-url", url), steps: tt.steps, tty: true})
			lines := strings.Split(strings.TrimSuffix(res.Stderr, "\n"), "\n")
			last := lines[len(lines)-1]
			if tt.want == "" && res.Stderr != "" {
				t.Errorf("stderr = %q, want nothing", res.Stderr)
			}
			if tt.want != "" && !strings.HasPrefix(last, "chronos: "+tt.want+" "+api.URL+"/••••") {
				t.Errorf("last line = %q, want %q", last, tt.want)
			}
		})
	}
	t.Run("--no-ping-body says so", func(t *testing.T) {
		api, url := pingServer(t, pinged())
		res := runExec(t, execRun{flags: []string{"--no-ping-body", "--ping-url", url}, steps: []string{"exit:0"}, tty: true})
		if want := "chronos: sent exit code 0 to " + api.URL + "/••••, without the output.\n"; res.Stderr != want {
			t.Errorf("stderr = %q, want %q", res.Stderr, want)
		}
	})
	t.Run("not started", func(t *testing.T) {
		api, url := pingServer(t, pinged())
		res := clitest.Run(t, clitest.Options{Args: []string{"monitors", "exec", "--ping-url", url, "chronos-no-such-command"}, StderrTTY: true})
		if want := "chronos: sent exit code 127 to " + api.URL + "/•••• with this error.\n"; !strings.HasSuffix(res.Stderr, want) {
			t.Errorf("stderr = %q, want it to end %q", res.Stderr, want)
		}
	})
}

func TestExecUsageErrors(t *testing.T) {
	_, url := pingServer(t)
	for _, args := range [][]string{
		{"monitors", "exec", "--ping-url", url},
		{"monitors", "exec", "--ping-ulr", url, "--", "true"},
	} {
		res := clitest.Run(t, clitest.Options{Args: args})
		clitest.WantExit(t, res, exit.Usage)
		if !strings.HasPrefix(res.Stderr, "Error: ") {
			t.Errorf("%v: stderr = %q", args, res.Stderr)
		}
	}
	res := clitest.Run(t, clitest.Options{Args: []string{"monitors", "exec"}})
	if want := "Error: No command to run. Put it after the flags, such as chronos monitors exec -- ./backup.sh.\nRun chronos monitors exec --help for usage.\n"; res.Stderr != want {
		t.Errorf("stderr = %q, want %q", res.Stderr, want)
	}
}

func TestExecHelp(t *testing.T) {
	res := clitest.Run(t, clitest.Options{Args: []string{"monitors", "exec", "--help"}})
	clitest.WantExit(t, res, exit.OK)
	clitest.Golden(t, "exec_help", res.Stdout)
}

func TestForwardingRule(t *testing.T) {
	tests := []struct {
		sig                   os.Signal
		withTerminal, without bool
	}{
		{syscall.SIGTERM, true, true},
		{syscall.SIGINT, false, true},
		{syscall.SIGQUIT, false, true},
		{syscall.SIGHUP, false, true},
		{syscall.SIGPIPE, false, false},
	}
	for _, tt := range tests {
		if got := monitors.Forwards(tt.sig, true); got != tt.withTerminal {
			t.Errorf("%v with a controlling terminal: forwarded = %v", tt.sig, got)
		}
		if got := monitors.Forwards(tt.sig, false); got != tt.without {
			t.Errorf("%v without a controlling terminal: forwarded = %v", tt.sig, got)
		}
	}
}
