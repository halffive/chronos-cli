package root_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/cmd/root"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/output"
)

// probe adds test commands: "probe read <id>" (a GET), "probe write" (a
// POST with a push header) and "probe lookup-write" (a GET, then a POST).
func probe(f *app.Factory, root *cobra.Command) {
	client := func() (*chronos.Client, error) { return f.Client() }
	create := func(ctx context.Context, c *chronos.Client) (output.View, error) {
		s, err := c.Schedules.Create(ctx, chronos.ScheduleCreateParams{
			Name: "Nightly report",
			Cron: new("0 3 * * *"),
			Delivery: &chronos.Delivery{Type: chronos.DeliveryTypePush, HTTP: &chronos.HTTPDelivery{
				URL: "https://example.com/hook", Method: "POST", Headers: map[string]string{"Authorization": "Bearer downstream-secret"},
			}},
		})
		if err != nil {
			return nil, err
		}
		return output.Resource(s, func(*chronos.Schedule) []output.Field { return nil }), nil
	}

	read := cmdutil.NewCommand(f, cmdutil.CommandSpec{Use: "read <id>", Args: cmdutil.ExactArgs("id")}, func(ctx context.Context, _ *cobra.Command, args []string) (output.View, error) {
		c, err := client()
		if err != nil {
			return nil, err
		}
		s, err := c.Schedules.Get(ctx, args[0])
		if err != nil {
			return nil, err
		}
		return output.Resource(s, func(s *chronos.Schedule) []output.Field {
			return []output.Field{{Label: "Name", Value: s.Name}}
		}), nil
	})
	read.Flags().String("label", "", "")
	write := cmdutil.NewCommand(f, cmdutil.CommandSpec{Use: "write", Mutates: true}, func(ctx context.Context, _ *cobra.Command, _ []string) (output.View, error) {
		c, err := client()
		if err != nil {
			return nil, err
		}
		return create(ctx, c)
	})
	write.Flags().Int("page", 1, "")
	write.Flags().Bool("all", false, "")
	write.MarkFlagsMutuallyExclusive("page", "all")
	lookupWrite := cmdutil.NewCommand(f, cmdutil.CommandSpec{Use: "lookup-write", Mutates: true}, func(ctx context.Context, _ *cobra.Command, _ []string) (output.View, error) {
		c, err := client()
		if err != nil {
			return nil, err
		}
		if _, err := c.Schedules.Get(ctx, "s1"); err != nil {
			return nil, err
		}
		return create(ctx, c)
	})

	group := cmdutil.NewGroup("probe", "Test commands")
	group.AddCommand(read, write, lookupWrite)
	root.AddCommand(group)
}

func runProbe(t *testing.T, env map[string]string, args ...string) clitest.Result {
	t.Helper()
	return clitest.Run(t, clitest.Options{Args: args, Env: env, Extend: probe})
}

func TestExitCodesForAPIFailures(t *testing.T) {
	tests := []struct {
		name      string
		response  clitest.Response
		args      []string
		exit      exit.Code
		code      string
		retryable bool
	}{
		{"unauthorized", clitest.Failure{Status: 401, Code: "unauthorized", Message: "Unauthorized"}.Response(), []string{"read", "s1"}, exit.Auth, "unauthorized", false},
		{"not found", clitest.Failure{Status: 404, Code: "schedule_not_found", Message: "Schedule not found"}.Response(), []string{"read", "s1"}, exit.NotFound, "schedule_not_found", false},
		{"conflict", clitest.Failure{Status: 409, Code: "schedule_already_paused", Message: "Schedule is already paused"}.Response(), []string{"write"}, exit.Rejected, "schedule_already_paused", false},
		{"plan limit", clitest.Failure{Status: 403, Code: "plan_limit_exceeded", Message: "Plan limit reached"}.Response(), []string{"write"}, exit.Rejected, "plan_limit_exceeded", false},
		{"bad request", clitest.Failure{Status: 400, Code: "handler_required_for_pull", Message: "Pull delivery needs a handler"}.Response(), []string{"write"}, exit.Rejected, "handler_required_for_pull", false},
		{"rate limited write", clitest.Failure{Status: 429, Code: "rate_limit_exceeded", Message: "Too many requests"}.Response(), []string{"write"}, exit.Retryable, "rate_limit_exceeded", true},
		{"read 503", clitest.Failure{Status: 503, Code: "error", Message: "Service unavailable"}.Response(), []string{"read", "s1"}, exit.Retryable, "error", true},
		{"write 503", clitest.Failure{Status: 503, Code: "error", Message: "Service unavailable"}.Response(), []string{"write"}, exit.OutcomeUnknown, "outcome_unknown", false},
		{"write 500", clitest.Failure{Status: 500, Code: "internal_error", Message: "An unknown error occurred"}.Response(), []string{"write"}, exit.OutcomeUnknown, "outcome_unknown", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := clitest.NewAPI(t, tt.response)
			res := runProbe(t, api.Env(), append([]string{"--json", "probe"}, tt.args...)...)
			if res.Code != tt.exit {
				t.Fatalf("exit = %d, want %d\nstderr: %s", res.Code, tt.exit, res.Stderr)
			}
			if res.Stdout != "" {
				t.Errorf("stdout = %q, want nothing", res.Stdout)
			}
			got := clitest.DecodeError(t, res.Stderr)
			if got.Code != tt.code || got.Retryable != tt.retryable {
				t.Errorf("code = %q retryable = %v, want %q %v", got.Code, got.Retryable, tt.code, tt.retryable)
			}
			if got.Status == nil || *got.Status != tt.response.Status {
				t.Errorf("status = %v, want %d", got.Status, tt.response.Status)
			}
			if got.RequestID == nil || *got.RequestID != "req_test" {
				t.Errorf("request_id = %v, want req_test", got.RequestID)
			}
		})
	}
}

func TestValidationErrorJSON(t *testing.T) {
	fieldErrors := `[
		{"message":"\"delivery.http.url\" must be a valid uri","field":"delivery.http.url","value":"nope"},
		{"message":"provide either cron or interval, not both","field":"cron, interval"},
		{"message":"\"value\" must have at least 1 key","field":null}
	]`
	api := clitest.NewAPI(t, clitest.Failure{Status: 422, Code: "validation_error", Message: "request body failed validation", FieldErrors: fieldErrors}.Response())
	res := runProbe(t, api.Env(), "--json", "probe", "write")
	if res.Code != exit.Rejected {
		t.Fatalf("exit = %d, want %d", res.Code, exit.Rejected)
	}
	clitest.Golden(t, "validation_error", res.Stderr)
}

func TestValidationErrorForPeople(t *testing.T) {
	fieldErrors := `[{"message":"\"delivery.http.url\" must be a valid uri","field":"delivery.http.url"},{"message":"\"value\" must have at least 1 key","field":null}]`
	api := clitest.NewAPI(t, clitest.Failure{Status: 422, Code: "validation_error", Message: "request body failed validation", FieldErrors: fieldErrors}.Response())
	res := runProbe(t, api.Env(), "probe", "write")
	clitest.Golden(t, "validation_error_human", res.Stderr)
}

func TestNetworkFailures(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	refused := map[string]string{"CHRONOS_API_KEY": clitest.APIKey, "CHRONOS_BASE_URL": closed.URL}

	t.Run("refused write never left, so it is safe to repeat", func(t *testing.T) {
		res := runProbe(t, refused, "--json", "probe", "write")
		if got := clitest.DecodeError(t, res.Stderr); res.Code != exit.Retryable || got.Code != exit.CodeNetwork || !got.Retryable {
			t.Errorf("exit = %d, error = %+v; want %d %s retryable", res.Code, got, exit.Retryable, exit.CodeNetwork)
		}
	})
	t.Run("refused read is safe to repeat", func(t *testing.T) {
		res := runProbe(t, refused, "--json", "probe", "read", "s1")
		if res.Code != exit.Retryable {
			t.Errorf("exit = %d, want %d", res.Code, exit.Retryable)
		}
	})
	t.Run("write that timed out may have landed", func(t *testing.T) {
		api := clitest.NewAPI(t, clitest.Response{Status: 201, Body: "{}", Delay: time.Second})
		res := clitest.Run(t, clitest.Options{
			Args:          []string{"--json", "probe", "write"},
			Env:           api.Env(),
			Extend:        probe,
			ClientOptions: []chronos.Option{chronos.WithRequestTimeout(50 * time.Millisecond)},
		})
		got := clitest.DecodeError(t, res.Stderr)
		if res.Code != exit.OutcomeUnknown || got.Code != exit.CodeOutcomeUnknown || got.Retryable {
			t.Errorf("exit = %d, error = %+v; want %d %s", res.Code, got, exit.OutcomeUnknown, exit.CodeOutcomeUnknown)
		}
		if len(api.Requests()) != 1 {
			t.Errorf("requests = %d, want 1", len(api.Requests()))
		}
	})
}

func TestInterrupted(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Response{Status: 200, Body: "{}", Delay: time.Second})
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(50*time.Millisecond, cancel)
	res := clitest.Run(t, clitest.Options{Args: []string{"--json", "probe", "read", "s1"}, Env: api.Env(), Extend: probe, Context: ctx})
	if res.Code != exit.Interrupted {
		t.Errorf("exit = %d, want %d\nstderr: %s", res.Code, exit.Interrupted, res.Stderr)
	}
}

func TestMissingKey(t *testing.T) {
	res := runProbe(t, nil, "--json", "probe", "read", "s1")
	if got := clitest.DecodeError(t, res.Stderr); res.Code != exit.Auth || got.Code != exit.CodeAPIKeyMissing {
		t.Errorf("exit = %d, code = %q; want %d %s", res.Code, got.Code, exit.Auth, exit.CodeAPIKeyMissing)
	}
}

func TestDryRunSendsNothingAndNeedsNoKey(t *testing.T) {
	api := clitest.NewAPI(t, clitest.OK(clitest.Fixture("schedule")))
	res := runProbe(t, map[string]string{"CHRONOS_BASE_URL": api.URL}, "--json", "probe", "write", "--dry-run")
	if res.Code != exit.OK {
		t.Fatalf("exit = %d, stderr: %s", res.Code, res.Stderr)
	}
	if n := len(api.Requests()); n != 0 {
		t.Errorf("the API received %d requests, want 0", n)
	}
	want := `{"dry_run":{"method":"POST","url":"` + api.URL + `/v1/schedules","body":{"cron":"0 3 * * *","delivery":{"http":{"headers":{"Authorization":"••••"},"method":"POST","url":"https://example.com/hook"},"type":"push"},"name":"Nightly report"}}}` + "\n"
	if res.Stdout != want {
		t.Errorf("stdout = %s\nwant     %s", res.Stdout, want)
	}
}

func TestDryRunNeverReadsTheKeychain(t *testing.T) {
	api := clitest.NewAPI(t)
	dir := t.TempDir()
	saved := &config.File{Version: 1, Profiles: map[string]config.Profile{config.DefaultProfile: {
		BaseURL:    api.URL,
		Credential: config.Credential{Type: config.CredentialAPIKey, Storage: config.StorageKeychain},
	}}}
	if err := config.Save(dir, saved); err != nil {
		t.Fatal(err)
	}
	res := clitest.Run(t, clitest.Options{
		Args:     []string{"--json", "probe", "write", "--dry-run"},
		Env:      map[string]string{"CHRONOS_CONFIG_DIR": dir},
		Extend:   probe,
		Keychain: &clitest.Keychain{Err: errors.New("locked")},
	})
	if res.Code != exit.OK || !strings.Contains(res.Stdout, `"dry_run"`) {
		t.Errorf("exit = %d, stdout = %s, stderr = %s", res.Code, res.Stdout, res.Stderr)
	}
}

func TestUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"unknown flag", []string{"--json", "probe", "read", "s1", "--nope"}},
		{"missing argument", []string{"--json", "probe", "read"}},
		{"unknown command", []string{"--json", "probe", "nope"}},
		{"unknown top-level command", []string{"--json", "nope"}},
		{"invalid id", []string{"--json", "probe", "read", ".."}},
		{"conflicting flags", []string{"--json", "probe", "write", "--page", "2", "--all"}},
		{"argument to a help topic", []string{"--json", "exit-codes", "extra"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := runProbe(t, map[string]string{"CHRONOS_API_KEY": clitest.APIKey}, tt.args...)
			if got := clitest.DecodeError(t, res.Stderr); res.Code != exit.Usage || got.Code != exit.CodeUsage {
				t.Errorf("exit = %d, code = %q; want %d %s", res.Code, got.Code, exit.Usage, exit.CodeUsage)
			}
		})
	}
}

func TestUserAgent(t *testing.T) {
	api := clitest.NewAPI(t, clitest.OK(clitest.Fixture("schedule")))
	runProbe(t, api.Env(), "probe", "read", "s1")
	ua := api.Requests()[0].Header.Get("User-Agent")
	if !strings.HasPrefix(ua, "chronos-cli/") || !strings.Contains(ua, " chronos-go/"+chronos.Version) {
		t.Errorf("User-Agent = %q", ua)
	}
}

func TestEveryCommandFollowsTheContract(t *testing.T) {
	f := &app.Factory{IO: &app.IOStreams{}, Getenv: func(string) string { return "" }}
	root := rootCmd(f)
	mutatingVerbs := map[string]bool{
		"create": true, "update": true, "delete": true, "pause": true, "resume": true,
		"trigger": true, "archive": true, "cancel": true, "skip": true, "retry": true,
	}
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			walk(child)
		}
		if !cmd.HasParent() || cmd.Annotations[cmdutil.AnnotationGroup] != "" || isBuiltin(cmd) {
			return
		}
		if cmd.Annotations[cmdutil.AnnotationLeaf] == "" {
			t.Errorf("%s is not built with cmdutil.NewCommand", cmd.CommandPath())
		}
		if cmd.Flag("json") == nil {
			t.Errorf("%s has no --json", cmd.CommandPath())
		}
		if mutatingVerbs[cmd.Name()] && cmd.Flags().Lookup("dry-run") == nil {
			t.Errorf("%s changes state but has no --dry-run; set Mutates", cmd.CommandPath())
		}
	}
	walk(root)
}

func rootCmd(f *app.Factory) *cobra.Command {
	cmd := root.NewCmd(f)
	cmd.InitDefaultHelpCmd()
	cmd.InitDefaultCompletionCmd()
	return cmd
}

// isBuiltin reports cobra's own help and completion commands.
func isBuiltin(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "help" || c.Name() == "completion" {
			return true
		}
	}
	return false
}

func TestVersionJSON(t *testing.T) {
	res := clitest.Run(t, clitest.Options{Args: []string{"version", "--json"}})
	var got map[string]any
	if err := json.Unmarshal([]byte(res.Stdout), &got); err != nil || res.Code != exit.OK {
		t.Fatalf("exit %d, stdout %q: %v", res.Code, res.Stdout, err)
	}
	for _, key := range []string{"version", "commit", "built_at", "go_version", "sdk_version"} {
		if _, ok := got[key]; !ok {
			t.Errorf("version JSON has no %q: %s", key, res.Stdout)
		}
	}
}

func TestDryRunCapturesTheFirstRequestWithoutRetrying(t *testing.T) {
	api := clitest.NewAPI(t)
	start := time.Now()
	res := clitest.Run(t, clitest.Options{
		Args:          []string{"--json", "probe", "lookup-write", "--dry-run"},
		Env:           map[string]string{"CHRONOS_BASE_URL": api.URL},
		Extend:        probe,
		ClientOptions: []chronos.Option{chronos.WithMaxRequestRetries(2)},
	})
	if res.Code != exit.OK || !strings.Contains(res.Stdout, `"method":"GET"`) {
		t.Fatalf("exit %d, stdout %s, stderr %s", res.Code, res.Stdout, res.Stderr)
	}
	if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
		t.Errorf("dry run took %s; the captured request was retried", elapsed)
	}
}

func TestWriteWithUnusableSuccessIsOutcomeUnknown(t *testing.T) {
	for name, response := range map[string]clitest.Response{
		"html body":        {Status: 201, Body: "<html>gateway</html>"},
		"success is false": {Status: 200, Body: `{"success":false,"message":"odd","code":"error","errors":[]}`},
	} {
		t.Run(name, func(t *testing.T) {
			api := clitest.NewAPI(t, response)
			res := runProbe(t, api.Env(), "--json", "probe", "write")
			if res.Code != exit.OutcomeUnknown {
				t.Errorf("exit = %d, want %d\nstderr: %s", res.Code, exit.OutcomeUnknown, res.Stderr)
			}
		})
	}
}

func TestInterruptedWriteSaysTheOutcomeIsUnknown(t *testing.T) {
	api := clitest.NewAPI(t, clitest.Response{Status: 201, Body: "{}", Delay: time.Second})
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(50*time.Millisecond, cancel)
	res := clitest.Run(t, clitest.Options{Args: []string{"--json", "probe", "write"}, Env: api.Env(), Extend: probe, Context: ctx})
	if got := clitest.DecodeError(t, res.Stderr); res.Code != exit.Interrupted || !strings.Contains(got.Message, "may have reached the API") {
		t.Errorf("exit = %d, message = %q", res.Code, got.Message)
	}
}

func TestSavedKeyStaysWithItsBaseURL(t *testing.T) {
	api := clitest.NewAPI(t)
	dir := t.TempDir()
	saved := &config.File{Version: 1, Profiles: map[string]config.Profile{config.DefaultProfile: {
		Credential: config.Credential{Type: config.CredentialAPIKey, Storage: config.StorageFile, Secret: "chrns_saved"},
	}}}
	if err := config.Save(dir, saved); err != nil {
		t.Fatal(err)
	}
	res := runProbe(t, map[string]string{"CHRONOS_CONFIG_DIR": dir, "CHRONOS_BASE_URL": api.URL}, "--json", "probe", "read", "s1")
	got := clitest.DecodeError(t, res.Stderr)
	if res.Code != exit.Auth || got.Code != exit.CodeAPIKeyMissing || !strings.Contains(got.Message, "https://api.chronos.sh") {
		t.Errorf("exit = %d, error = %+v", res.Code, got)
	}
}

func TestPlainHTTPOnlyForLocalhost(t *testing.T) {
	res := runProbe(t, map[string]string{"CHRONOS_API_KEY": clitest.APIKey, "CHRONOS_BASE_URL": "http://api.chronos.sh"}, "--json", "probe", "read", "s1")
	if res.Code != exit.Usage {
		t.Errorf("exit = %d, want %d\nstderr: %s", res.Code, exit.Usage, res.Stderr)
	}
}

func TestEnvKeyIgnoresABrokenConfigFile(t *testing.T) {
	api := clitest.NewAPI(t, clitest.OK(clitest.Fixture("schedule")))
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("{not json"), 0o600)
	env := api.Env()
	env["CHRONOS_CONFIG_DIR"] = dir
	if res := runProbe(t, env, "probe", "read", "s1"); res.Code != exit.OK {
		t.Errorf("exit = %d, stderr: %s", res.Code, res.Stderr)
	}
}

func TestJSONIsReadFromTheParsedFlags(t *testing.T) {
	res := runProbe(t, nil, "probe", "read", "s1", "--label", "--json")
	if res.Code != exit.Auth || strings.HasPrefix(res.Stderr, "{") {
		t.Errorf("--json given as a flag value printed a JSON error: exit %d, stderr %q", res.Code, res.Stderr)
	}
}

func TestExitCodesHelp(t *testing.T) {
	res := clitest.Run(t, clitest.Options{Args: []string{"help", "exit-codes"}})
	clitest.Golden(t, "exit_codes", res.Stdout)
}

func TestHelpTopicsAsJSON(t *testing.T) {
	for _, name := range []string{"exit-codes", "environment"} {
		res := clitest.Run(t, clitest.Options{Args: []string{name, "--json"}})
		var got map[string][]map[string]any
		if err := json.Unmarshal([]byte(res.Stdout), &got); err != nil || res.Code != exit.OK || len(got) != 1 {
			t.Errorf("%s --json: exit %d, stdout %q, %v", name, res.Code, res.Stdout, err)
		}
	}
}

func TestFlagErrorKeepsAnExplicitJSON(t *testing.T) {
	for _, args := range [][]string{
		{"--json", "version", "--base-url", "--json=false", "--nope"},
		{"--json", "version", "--json=nonsense"},
	} {
		res := clitest.Run(t, clitest.Options{Args: args})
		if res.Code != exit.Usage || !strings.HasPrefix(res.Stderr, `{"error"`) {
			t.Errorf("%v: exit %d, stderr %q", args, res.Code, res.Stderr)
		}
	}
}
