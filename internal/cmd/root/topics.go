package root

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/output"
)

// scriptsFooter ends the root help only; subcommands inherit the template.
const scriptsFooter = `{{if not .HasParent}}
Scripts and agents:
  Set CHRONOS_API_KEY to use a key without logging in.
  For JSON on stdout, add --json to a subcommand of schedules, jobs, executions or auth,
  or to version, exit-codes or environment. Errors then go to stderr as {"error": {...}}.
  Schedules and jobs commands that change something accept --dry-run, which prints the request and sends nothing.
  chronos prompts only in a terminal and never with --json. Pass --yes to confirm a delete.
  Run chronos help exit-codes to see what each exit code means.
  JSON output, exit codes, error codes, flag names and environment variables stay compatible.
  Tables and tab-separated output may change.
{{end}}`

type exitCodeJSON struct {
	Code    exit.Code `json:"code"`
	Meaning string    `json:"meaning"`
}

type variableJSON struct {
	Name    string `json:"name"`
	Meaning string `json:"meaning"`
}

var variables = []variableJSON{
	{"CHRONOS_API_KEY", "API key. Overrides the key saved by chronos auth login."},
	{"CHRONOS_BASE_URL", "API base URL. Defaults to https://api.chronos.sh. A saved key is only sent to the base URL it was saved for."},
	{"CHRONOS_CONFIG_DIR", "Where chronos saves its config. Defaults to $XDG_CONFIG_HOME/chronos, then ~/.config/chronos, or the user config directory on Windows."},
	{"NO_COLOR", "Set to any value to turn off color."},
}

func exitCodesTopic(f *app.Factory) *cobra.Command {
	var codes []exitCodeJSON
	var b strings.Builder
	b.WriteString("chronos exits with one of these codes. With --json, the error object on stderr\nalso has a \"code\" string and a \"retryable\" boolean, which is true when\nrunning the same command again is safe.\n\n")
	for _, d := range exit.Codes() {
		codes = append(codes, exitCodeJSON{d.Code, d.Meaning})
		fmt.Fprintf(&b, "  %-4d %s\n", d.Code, d.Meaning)
	}
	return newTopic(f, topic{name: "exit-codes", short: "What each exit code means", text: b.String(), value: map[string]any{"exit_codes": codes}})
}

func environmentTopic(f *app.Factory) *cobra.Command {
	var b strings.Builder
	for _, v := range variables {
		fmt.Fprintf(&b, "%-20s %s\n", v.Name, v.Meaning)
	}
	return newTopic(f, topic{name: "environment", short: "Environment variables chronos reads", text: b.String(), value: map[string]any{"variables": variables}})
}

type topic struct {
	name, short, text string
	value             any
}

// newTopic builds a help topic: text for chronos help <name>, and JSON for
// chronos <name> --json.
func newTopic(f *app.Factory, t topic) *cobra.Command {
	cmd := cmdutil.NewCommand(f, cmdutil.CommandSpec{Use: t.name, Short: t.short, Long: t.text}, func(context.Context, *cobra.Command, []string) (output.View, error) {
		return output.ViewOf(t.value, func(p *output.Printer) error {
			_, err := fmt.Fprint(p.Out, t.text)
			return err
		}), nil
	})
	cmd.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		fmt.Fprint(cmd.OutOrStdout(), t.text)
	})
	return cmd
}
