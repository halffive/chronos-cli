// Package cmdutil builds cobra commands on top of app: leaf commands that
// print a view and report errors the same way, command groups, and argument
// checks.
package cmdutil

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/output"
	"chronos.sh/cli/internal/transport"
)

// Annotations that let tests check every command is built the same way.
const (
	AnnotationLeaf  = "chronos.leaf"
	AnnotationGroup = "chronos.group"
)

// CommandSpec describes a command. Args defaults to no arguments.
type CommandSpec struct {
	Use     string
	Short   string
	Long    string
	Example string
	Args    cobra.PositionalArgs
	// Mutates adds --dry-run, which captures the command's first request
	// instead of sending it. Check f.DryRun to skip any read that would
	// otherwise come first, such as a lookup for a confirmation prompt.
	Mutates bool
}

// RunFunc runs a command. A view returned with an error is still printed, so
// a failed job reaches stdout along with exit 8.
type RunFunc func(ctx context.Context, cmd *cobra.Command, args []string) (output.View, error)

// NewCommand builds a leaf command that prints run's view and returns run's
// error to root.Execute.
func NewCommand(f *app.Factory, spec CommandSpec, run RunFunc) *cobra.Command {
	args := spec.Args
	if args == nil {
		args = cobra.NoArgs
	}
	cmd := &cobra.Command{
		Use:         spec.Use,
		Short:       spec.Short,
		Long:        spec.Long,
		Example:     spec.Example,
		Args:        UsageArgs(args),
		Annotations: map[string]string{AnnotationLeaf: "true"},
		// Cobra checks required flags and flag groups after PreRunE with
		// plain errors; checking them here first makes them usage errors.
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.ValidateRequiredFlags(); err != nil {
				return exit.Usagef("%s", exit.Sentence(err.Error()))
			}
			if err := cmd.ValidateFlagGroups(); err != nil {
				return exit.Usagef("%s", exit.Sentence(err.Error()))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			view, err := run(cmd.Context(), cmd, args)
			p := f.Printer()
			if f.DryRun {
				return finishDryRun(p, err)
			}
			if view == nil {
				return err
			}
			if printErr := p.Print(view); printErr != nil && err == nil {
				return printErr
			}
			if err == nil {
				printNextPage(p, cmd, args, view)
			}
			return err
		},
	}
	if spec.Mutates {
		cmd.Flags().BoolVar(&f.DryRun, "dry-run", false, "Print the request instead of sending it")
	}
	return cmd
}

func finishDryRun(p *output.Printer, err error) error {
	if captured, ok := errors.AsType[*transport.Captured](err); ok {
		return p.Print(app.DryRunView(captured))
	}
	if err == nil {
		return exit.Failuref("This command sent no request, so the dry run has nothing to show.")
	}
	return err
}

// NewGroup builds a command that only holds subcommands, such as schedules.
func NewGroup(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        GroupArgs,
		Annotations: map[string]string{AnnotationGroup: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
}

// GroupArgs rejects an unknown subcommand as a usage error.
func GroupArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	msg := fmt.Sprintf("Unknown command %q for %q.", args[0], cmd.CommandPath())
	if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
		msg += fmt.Sprintf(" Did you mean %s?", strings.Join(suggestions, " or "))
	}
	return exit.Usagef("%s", msg)
}

// ExactArgs requires one positional argument per name, such as ExactArgs("id").
func ExactArgs(names ...string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == len(names) {
			return nil
		}
		want := "<" + strings.Join(names, "> <") + ">"
		return exit.Usagef("%s takes %s. You passed %d.", cmd.CommandPath(), want, len(args))
	}
}

// UsageArgs makes any argument check fail as a usage error.
func UsageArgs(check cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		err := check(cmd, args)
		if err == nil {
			return nil
		}
		if _, ok := errors.AsType[*exit.Error](err); ok {
			return err
		}
		return exit.Usagef("%s", exit.Sentence(err.Error()))
	}
}

// CompleteValues completes a flag from a fixed list, such as statuses.
func CompleteValues[T ~string](values ...T) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	completions := make([]string, len(values))
	for i, v := range values {
		completions[i] = string(v)
	}
	return func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completions, cobra.ShellCompDirectiveNoFileComp
	}
}

type paged interface {
	Paging() chronos.Paging
}

func printNextPage(p *output.Printer, cmd *cobra.Command, args []string, view output.View) {
	pv, ok := view.(paged)
	if !ok || p.JSON || !p.TTY {
		return
	}
	paging := pv.Paging()
	if paging.Page >= paging.Pages {
		return
	}
	parts := []string{cmd.CommandPath()}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	cmd.Flags().Visit(func(fl *pflag.Flag) {
		if fl.Name != "page" {
			parts = append(parts, flagArgs(fl)...)
		}
	})
	parts = append(parts, "--page", strconv.Itoa(paging.Page+1))
	p.Note("\nNext page: %s", strings.Join(parts, " "))
}

// flagArgs writes a set flag back as arguments that set it the same way.
func flagArgs(fl *pflag.Flag) []string {
	name := "--" + fl.Name
	if slice, ok := fl.Value.(pflag.SliceValue); ok {
		var args []string
		for _, v := range slice.GetSlice() {
			args = append(args, name, shellQuote(v))
		}
		return args
	}
	if fl.Value.Type() == "bool" {
		if fl.Value.String() == "true" {
			return []string{name}
		}
		return []string{name + "=" + fl.Value.String()}
	}
	return []string{name, shellQuote(fl.Value.String())}
}

// shellQuote leaves a value bare only when it is made of characters no shell
// treats specially.
func shellQuote(s string) string {
	if s != "" && !strings.ContainsFunc(s, needsQuoting) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func needsQuoting(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return false
	}
	return !strings.ContainsRune("._/:@%+-,", r)
}
