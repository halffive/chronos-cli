package cmdutil

import (
	"fmt"
	"slices"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/input"
)

var pushFlagNames = []string{"push-url", "push-method", "push-header"}

type PushFlags struct {
	flags   *pflag.FlagSet
	url     string
	method  string
	headers []string
}

func (p *PushFlags) Register(cmd *cobra.Command) {
	p.flags = cmd.Flags()
	p.flags.StringVar(&p.url, "push-url", "", "Push to this URL instead of waiting for a worker")
	p.flags.StringVar(&p.method, "push-method", "", "HTTP method for push delivery (default POST)")
	p.flags.StringArrayVar(&p.headers, "push-header", nil, "Header sent with each push, as 'Name: value' (repeatable)")
	_ = cmd.RegisterFlagCompletionFunc("push-method", CompleteValues("GET", "POST", "PUT", "PATCH", "DELETE"))
}

// Parse returns what the flags set, or nil when none was given.
func (p *PushFlags) Parse() (*input.Push, error) {
	if !slices.ContainsFunc(pushFlagNames, p.flags.Changed) {
		return nil, nil
	}
	var push input.Push
	if p.flags.Changed("push-url") {
		push.URL = &p.url
	}
	if p.flags.Changed("push-method") {
		push.Method = &p.method
	}
	for _, header := range p.headers {
		if err := push.SetHeader(header); err != nil {
			return nil, err
		}
	}
	return &push, nil
}

func WaitTimeoutVar(cmd *cobra.Command, value *string) {
	cmd.Flags().StringVar(value, "wait-timeout", fmt.Sprintf("%ds", int(app.DefaultWaitTimeout.Seconds())), "How long to wait for the job to finish, such as 90s or 5m")
}

type WaitFlags struct {
	flags   *pflag.FlagSet
	Wait    bool
	timeout string
}

func (w *WaitFlags) Register(cmd *cobra.Command) {
	w.flags = cmd.Flags()
	w.flags.BoolVar(&w.Wait, "wait", false, "Wait for the job to finish and print it")
	WaitTimeoutVar(cmd, &w.timeout)
}

// Timeout checks the flags and returns the wait timeout, or 0 without
// --wait. Call it before the write, so a bad flag never leaves the write
// applied.
func (w *WaitFlags) Timeout() (time.Duration, error) {
	if w.Wait {
		return app.ParseWaitTimeout(w.timeout)
	}
	if w.flags.Changed("wait-timeout") {
		return 0, exit.Usagef("--wait-timeout only applies with --wait.")
	}
	return 0, nil
}

// TimeFilter is a flag value for input.ParseTimeFilter, resolved when the
// flag is parsed. It prints as RFC 3339, so the next-page hint asks for the
// same window instead of one measured from a later now.
type TimeFilter struct {
	target *time.Time
	now    func() time.Time
}

func NewTimeFilter(target *time.Time, now func() time.Time) *TimeFilter {
	return &TimeFilter{target: target, now: now}
}

func (v *TimeFilter) Set(s string) error {
	t, err := input.ParseTimeFilter(s, v.now())
	if err != nil {
		return err
	}
	*v.target = t
	return nil
}

func (v *TimeFilter) String() string {
	if v.target.IsZero() {
		return ""
	}
	return v.target.UTC().Format(time.RFC3339Nano)
}

func (v *TimeFilter) Type() string { return "time" }
