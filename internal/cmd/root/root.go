// Package root assembles the chronos command tree and owns the process's exit
// code.
package root

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/build"
	"chronos.sh/cli/internal/cmd/auth"
	"chronos.sh/cli/internal/cmd/executions"
	"chronos.sh/cli/internal/cmd/jobs"
	"chronos.sh/cli/internal/cmd/schedules"
	"chronos.sh/cli/internal/cmd/version"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/exit"
)

const (
	groupResources = "resources"
	groupSetup     = "setup"
	groupTopics    = "topics"
)

func NewCmd(f *app.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "chronos",
		Short:         "Manage Chronos from the terminal",
		Long:          "Create and run schedules, start one-off jobs, and see what ran and what failed.",
		Version:       build.Current().Version,
		Args:          cmdutil.GroupArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.SetVersionTemplate("chronos {{.Version}}\n")
	cmd.SetUsageTemplate(cmd.UsageTemplate() + scriptsFooter)
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &flagParseError{exit.Usagef("%s", exit.Sentence(err.Error()))}
	})

	cmd.PersistentFlags().BoolP("help", "h", false, "Show help for a command")
	cmd.Flags().BoolP("version", "v", false, "Show the chronos version")
	flags := cmd.PersistentFlags()
	flags.BoolVar(&f.JSON, "json", false, "Print JSON on stdout, and errors as JSON on stderr")
	flags.StringVar(&f.BaseURL, "base-url", "", "API base URL")
	_ = flags.MarkHidden("base-url")

	cmd.AddGroup(
		&cobra.Group{ID: groupResources, Title: "Resources:"},
		&cobra.Group{ID: groupSetup, Title: "Setup:"},
		&cobra.Group{ID: groupTopics, Title: "Help topics:"},
	)
	addToGroup(cmd, groupResources, schedules.NewCmd(f), jobs.NewCmd(f), executions.NewCmd(f))
	addToGroup(cmd, groupSetup, auth.NewCmd(f), version.NewCmd(f))
	addToGroup(cmd, groupTopics, exitCodesTopic(f), environmentTopic(f))
	cmd.SetCompletionCommandGroupID(groupSetup)
	cmd.SetHelpCommandGroupID(groupSetup)
	return cmd
}

func addToGroup(parent *cobra.Command, group string, children ...*cobra.Command) {
	for _, child := range children {
		child.GroupID = group
		parent.AddCommand(child)
	}
}

// flagParseError marks a failure to parse flags, after which --json may not
// have been read yet.
type flagParseError struct{ usage *exit.Error }

func (e *flagParseError) Error() string { return e.usage.Error() }
func (e *flagParseError) Unwrap() error { return e.usage }

func Run(ctx context.Context, f *app.Factory, args []string) exit.Code {
	return Execute(ctx, f, NewCmd(f), args)
}

// Execute runs cmd, a root command, and returns the exit code.
func Execute(ctx context.Context, f *app.Factory, cmd *cobra.Command, args []string) exit.Code {
	cmd.SetArgs(args)
	cmd.SetIn(f.IO.In)
	cmd.SetOut(f.IO.Out)
	cmd.SetErr(f.IO.ErrOut)

	executed, err := cmd.ExecuteContextC(ctx)
	if err == nil {
		return exit.OK
	}
	if _, ok := errors.AsType[*flagParseError](err); ok {
		f.JSON = f.JSON || wantsJSON(args)
	}
	outcome := exit.Classify(err, f.Sent())
	p := f.Printer()
	p.Error(outcome)
	if outcome.Exit == exit.Usage && executed != nil {
		p.Note("Run %s --help for usage.", executed.CommandPath())
	}
	return outcome.Exit
}

// wantsJSON reads --json from raw arguments, the way the flag parser does.
func wantsJSON(args []string) bool {
	json := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--json" {
			json = true
			continue
		}
		value, ok := strings.CutPrefix(arg, "--json=")
		if parsed, err := strconv.ParseBool(value); ok && err == nil {
			json = parsed
		}
	}
	return json
}
