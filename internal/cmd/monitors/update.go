package monitors

import (
	"context"
	"maps"
	"slices"
	"strings"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/output"
)

var clearable = map[string]func(*chronos.MonitorUpdateParams){
	"description": func(p *chronos.MonitorUpdateParams) { p.Description = chronos.Null[string]() },
}

func newUpdateCmd(f *app.Factory) *cobra.Command {
	var flags bodyFlags
	var cleared []string
	spec := cmdutil.CommandSpec{
		Use:   "update <id>",
		Short: "Change a monitor",
		Long: "Change only the fields you pass.\n\n" +
			"--file takes an API request body, and flags override its fields.",
		Example: `  chronos monitors update <id> --cron "0 4 * * *"
  chronos monitors update <id> --grace 15m --clear description`,
		Args:    cmdutil.ExactArgs("id"),
		Mutates: true,
	}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, cmd *cobra.Command, args []string) (output.View, error) {
		if err := checkClear(cmd.Flags(), cleared); err != nil {
			return nil, err
		}
		var params chronos.MonitorUpdateParams
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
		m, err := c.Monitors.Update(ctx, args[0], params)
		if err != nil {
			return nil, err
		}
		return monitorView(m, maskedPingURL), nil
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
