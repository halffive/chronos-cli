package version

import (
	"context"
	"fmt"
	"runtime"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/build"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/output"
)

type versionJSON struct {
	Version    string  `json:"version"`
	Commit     *string `json:"commit"`
	BuiltAt    *string `json:"built_at"`
	GoVersion  string  `json:"go_version"`
	SDKVersion string  `json:"sdk_version"`
}

func NewCmd(f *app.Factory) *cobra.Command {
	spec := cmdutil.CommandSpec{Use: "version", Short: "Show the chronos version"}
	return cmdutil.NewCommand(f, spec, func(context.Context, *cobra.Command, []string) (output.View, error) {
		current := build.Current()
		info := versionJSON{
			Version:    current.Version,
			Commit:     optional(current.Commit),
			BuiltAt:    optional(current.BuiltAt),
			GoVersion:  runtime.Version(),
			SDKVersion: chronos.Version,
		}
		return output.ViewOf(info, func(p *output.Printer) error {
			_, err := fmt.Fprintf(p.Out, "chronos %s (chronos-go %s, %s)\n", info.Version, info.SDKVersion, info.GoVersion)
			return err
		}), nil
	})
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
