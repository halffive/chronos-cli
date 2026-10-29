package app_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/output"
)

const jobID = "0192f3a2-0000-7000-8000-000000000003"

func job(status string) string {
	var j map[string]any
	json.Unmarshal([]byte(clitest.Fixture("job")), &j)
	j["status"] = status
	b, _ := json.Marshal(j)
	return string(b)
}

// waiter adds "wait-probe", which waits for the fixture job, and with
// --create first creates it.
func waiter(f *app.Factory, root *cobra.Command) {
	var create bool
	cmd := cmdutil.NewCommand(f, cmdutil.CommandSpec{Use: "wait-probe"}, func(ctx context.Context, _ *cobra.Command, _ []string) (output.View, error) {
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		known := &chronos.Job{ID: jobID, Status: chronos.JobStatusQueued}
		if create {
			if known, err = c.Jobs.Create(ctx, chronos.JobCreateParams{Name: "report"}); err != nil {
				return nil, err
			}
		}
		j, err := f.WaitForJob(ctx, c, app.WaitOptions{JobID: jobID, Timeout: time.Second, Job: known})
		if j == nil {
			return nil, err
		}
		return output.ViewOf(j, func(*output.Printer) error { return nil }), err
	})
	cmd.Flags().BoolVar(&create, "create", false, "")
	root.AddCommand(cmd)
}

func TestWaitForJob(t *testing.T) {
	unavailable := clitest.Failure{Status: 503, Code: "error", Message: "Service unavailable"}.Response()
	tests := []struct {
		name      string
		responses []clitest.Response
		args      []string
		exit      exit.Code
		code      string
		retryable bool
		status    string
	}{
		{name: "completed", responses: []clitest.Response{clitest.OK(job("running")), clitest.OK(job("completed"))}, exit: exit.OK, status: "completed"},
		{name: "failed", responses: []clitest.Response{clitest.OK(job("failed"))}, exit: exit.JobUnsuccessful, code: exit.CodeJobUnsuccessful, status: "failed"},
		{name: "skipped", responses: []clitest.Response{clitest.OK(job("skipped"))}, exit: exit.JobUnsuccessful, code: exit.CodeJobUnsuccessful, status: "skipped"},
		{name: "timed out on its own is safe to repeat", responses: []clitest.Response{clitest.OK(job("running"))}, exit: exit.WaitTimeout, code: exit.CodeWaitTimeout, retryable: true, status: "running"},
		{name: "timed out after a create is not", responses: []clitest.Response{{Status: 201, Body: `{"success":true,"message":"ok","data":` + job("queued") + `}`}, clitest.OK(job("running"))}, args: []string{"--create"}, exit: exit.WaitTimeout, code: exit.CodeWaitTimeout, status: "running"},
		{name: "check failed after a create", responses: []clitest.Response{{Status: 201, Body: `{"success":true,"message":"ok","data":` + job("queued") + `}`}, unavailable}, args: []string{"--create"}, exit: exit.WaitTimeout, code: exit.CodeWaitFailed, status: "queued"},
		{name: "check failed on its own", responses: []clitest.Response{unavailable}, exit: exit.Retryable, code: "error", retryable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := clitest.NewAPI(t, tt.responses...)
			res := clitest.Run(t, clitest.Options{Args: append([]string{"--json", "wait-probe"}, tt.args...), Env: api.Env(), Extend: waiter})
			if res.Code != tt.exit {
				t.Fatalf("exit = %d, want %d\nstderr: %s", res.Code, tt.exit, res.Stderr)
			}
			if tt.status != "" && !strings.Contains(res.Stdout, `"status":"`+tt.status+`"`) {
				t.Errorf("stdout = %s, want the job with status %s", res.Stdout, tt.status)
			}
			if tt.code == "" {
				return
			}
			var e struct{ Error exit.Outcome }
			json.Unmarshal([]byte(res.Stderr), &e)
			if e.Error.Code != tt.code || e.Error.Retryable != tt.retryable {
				t.Errorf("error = %+v, want code %s retryable %v", e.Error, tt.code, tt.retryable)
			}
		})
	}
}

func TestWaitFailedKeepsTheRequestID(t *testing.T) {
	api := clitest.NewAPI(t,
		clitest.Response{Status: 201, Body: `{"success":true,"message":"ok","data":` + job("queued") + `}`},
		clitest.Failure{Status: 503, Code: "error", Message: "Service unavailable"}.Response(),
	)
	res := clitest.Run(t, clitest.Options{Args: []string{"--json", "wait-probe", "--create"}, Env: api.Env(), Extend: waiter})
	if !strings.Contains(res.Stderr, `"status":503`) || !strings.Contains(res.Stderr, `"request_id":"req_test"`) {
		t.Errorf("stderr = %s", res.Stderr)
	}
}

func confirmer(f *app.Factory, root *cobra.Command) {
	var yes bool
	cmd := cmdutil.NewCommand(f, cmdutil.CommandSpec{Use: "confirm-probe"}, func(ctx context.Context, _ *cobra.Command, _ []string) (output.View, error) {
		if err := f.Confirm(ctx, "Delete schedule \"Nightly\x1b[8m\"?", yes); err != nil {
			return nil, err
		}
		return output.ViewOf("confirmed", func(p *output.Printer) error { return nil }), nil
	})
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "")
	root.AddCommand(cmd)
}

func TestConfirm(t *testing.T) {
	tests := []struct {
		name  string
		tty   bool
		stdin string
		args  []string
		exit  exit.Code
		code  string
	}{
		{name: "no terminal needs --yes", stdin: "y\n", exit: exit.Usage, code: exit.CodeUsage},
		{name: "no terminal with -y", args: []string{"-y"}, exit: exit.OK},
		{name: "json on a terminal needs --yes", tty: true, stdin: "y\n", args: []string{"--json"}, exit: exit.Usage, code: exit.CodeUsage},
		{name: "terminal answers yes", tty: true, stdin: "yes\n", exit: exit.OK},
		{name: "terminal answers no", tty: true, stdin: "n\n", exit: exit.Failure, code: exit.CodeCancelled},
		{name: "terminal closes stdin", tty: true, exit: exit.Failure, code: exit.CodeCancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := clitest.Run(t, clitest.Options{Args: append([]string{"--json=false", "confirm-probe"}, tt.args...), StdinTTY: tt.tty, StdoutTTY: tt.tty, Stdin: tt.stdin, Extend: confirmer})
			if res.Code != tt.exit {
				t.Fatalf("exit = %d, want %d\nstderr: %s", res.Code, tt.exit, res.Stderr)
			}
			if strings.Contains(res.Stderr, "\x1b") {
				t.Errorf("an escape sequence reached the terminal: %q", res.Stderr)
			}
			if tt.code == exit.CodeCancelled && !strings.Contains(res.Stderr, "Cancelled.") {
				t.Errorf("stderr = %q, want the cancelled message", res.Stderr)
			}
		})
	}
}

func TestCollectAll(t *testing.T) {
	items := func(n int) func(func(int, error) bool) {
		return func(yield func(int, error) bool) {
			for i := range n {
				if !yield(i, nil) {
					return
				}
			}
		}
	}
	page, err := app.CollectAll(items(3))
	if err != nil || len(page.Data) != 3 || page.Paging != (chronos.Paging{Page: 1, Pages: 1, Size: 3, Total: 3}) {
		t.Errorf("CollectAll(3) = %+v, %v", page, err)
	}
	page, _ = app.CollectAll(items(0))
	if b, _ := json.Marshal(page); string(b) != `{"data":[],"paging":{"page":1,"pages":0,"size":0,"total":0}}` {
		t.Errorf("CollectAll(0) = %s", b)
	}
}
