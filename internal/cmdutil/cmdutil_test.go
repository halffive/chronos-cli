package cmdutil_test

import (
	"context"
	"strings"
	"testing"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

func lister(f *app.Factory, root *cobra.Command) {
	var page int
	var status string
	cmd := cmdutil.NewCommand(f, cmdutil.CommandSpec{Use: "list-probe"}, func(ctx context.Context, _ *cobra.Command, _ []string) (output.View, error) {
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		list, err := c.Schedules.List(ctx, chronos.ScheduleListParams{Page: page, Status: chronos.ScheduleStatus(status)})
		if err != nil {
			return nil, err
		}
		return output.List(list, "schedules", []output.Column[*chronos.Schedule]{{Header: "ID", Value: func(s *chronos.Schedule) any { return s.ID }}}), nil
	})
	cmd.Flags().IntVar(&page, "page", 1, "")
	cmd.Flags().StringVar(&status, "status", "", "")
	cmd.Flags().Bool("archived", false, "")
	cmd.Flags().StringSlice("handler", nil, "")
	root.AddCommand(cmd)
}

func TestNextPageHint(t *testing.T) {
	list := "[" + clitest.Fixture("schedule") + "]"
	tests := []struct {
		name   string
		paging chronos.Paging
		args   []string
		want   string
	}{
		{
			name:   "keeps the filters",
			paging: chronos.Paging{Page: 2, Pages: 3, Size: 20, Total: 41},
			args:   []string{"--status", "paused", "--page", "2"},
			want:   "Next page: chronos list-probe --status paused --page 3",
		},
		{
			name:   "keeps an explicit false and every slice value",
			paging: chronos.Paging{Page: 1, Pages: 3, Size: 20, Total: 41},
			args:   []string{"--archived=false", "--handler", "a", "--handler", "b c", "--status", "#ops"},
			want:   "Next page: chronos list-probe --archived=false --handler a --handler 'b c' --status '#ops' --page 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := clitest.NewAPI(t, clitest.Page(list, tt.paging))
			res := clitest.Run(t, clitest.Options{Args: append([]string{"list-probe"}, tt.args...), Env: api.Env(), StdoutTTY: true, Extend: lister})
			if !strings.Contains(res.Stderr, tt.want) {
				t.Errorf("stderr = %q, want %q", res.Stderr, tt.want)
			}
		})
	}

	api := clitest.NewAPI(t, clitest.Page(list, chronos.Paging{Page: 3, Pages: 3, Size: 20, Total: 41}))
	res := clitest.Run(t, clitest.Options{Args: []string{"list-probe", "--page", "3"}, Env: api.Env(), StdoutTTY: true, Extend: lister})
	if strings.Contains(res.Stderr, "Next page") {
		t.Errorf("hint on the last page: %q", res.Stderr)
	}
}

func TestCompleteValues(t *testing.T) {
	complete := cmdutil.CompleteValues(chronos.ScheduleStatusActive, chronos.ScheduleStatusPaused)
	got, directive := complete(nil, nil, "")
	if strings.Join(got, ",") != "active,paused" || directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("got %v, %v", got, directive)
	}
}
