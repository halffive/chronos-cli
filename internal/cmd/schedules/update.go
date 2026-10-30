package schedules

import (
	"context"
	"maps"
	"slices"
	"strings"
	"time"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/output"
)

var clearable = map[string]func(*chronos.ScheduleUpdateParams){
	"description": func(p *chronos.ScheduleUpdateParams) { p.Description = chronos.Null[string]() },
	"handler":     func(p *chronos.ScheduleUpdateParams) { p.Handler = chronos.Null[string]() },
	"runs":        func(p *chronos.ScheduleUpdateParams) { p.Runs = chronos.Null[int]() },
	"stop-at":     func(p *chronos.ScheduleUpdateParams) { p.StopAt = chronos.Null[time.Time]() },
	"payload":     func(p *chronos.ScheduleUpdateParams) { p.Payload = chronos.Null[map[string]any]() },
}

func newUpdateCmd(f *app.Factory) *cobra.Command {
	var flags bodyFlags
	var cleared []string
	spec := cmdutil.CommandSpec{
		Use:   "update <id>",
		Short: "Change a schedule",
		Long: "Change only the fields you pass. --push-* flags replace the whole delivery setting, so pass --push-url with them.\n\n" +
			"--file takes an API request body, and flags override its fields.",
		Example: `  chronos schedules update <id> --cron "0 4 * * *"
  chronos schedules update <id> --clear runs --clear stop-at`,
		Args:    cmdutil.ExactArgs("id"),
		Mutates: true,
	}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, cmd *cobra.Command, args []string) (output.View, error) {
		if err := checkClear(cmd.Flags(), cleared); err != nil {
			return nil, err
		}
		var params chronos.ScheduleUpdateParams
		b, err := flags.read(cmd.Flags(), f.IO.In, &params)
		if err != nil {
			return nil, err
		}
		b.applyUpdate(&params)
		for _, field := range cleared {
			clearable[field](&params)
		}
		c, err := f.Client()
		if err != nil {
			return nil, err
		}
		s, err := c.Schedules.Update(ctx, args[0], params)
		if err != nil {
			return nil, err
		}
		return scheduleView(s), nil
	})
	flags.register(cmd)
	cmd.Flags().StringSliceVar(&cleared, "clear", nil, "Unset a field, one of "+strings.Join(clearableFields(), ", ")+" (repeatable)")
	_ = cmd.RegisterFlagCompletionFunc("clear", cmdutil.CompleteValues(clearableFields()...))
	return cmd
}

func clearableFields() []string {
	return slices.Sorted(maps.Keys(clearable))
}

func checkClear(flags *pflag.FlagSet, fields []string) error {
	for _, field := range fields {
		if clearable[field] == nil {
			return exit.Usagef("--clear takes one of %s, not %q.", strings.Join(clearableFields(), ", "), field)
		}
		if flags.Changed(field) {
			return exit.Usagef("--%s and --clear %s conflict. Pass one of them.", field, field)
		}
	}
	return nil
}
