// Package build reports which build of the CLI is running.
package build

import (
	"runtime/debug"
	"strings"
)

// Set with -ldflags -X, so these must stay variables.
var (
	version = ""
	commit  = ""
	builtAt = ""
)

// Info identifies a build. Commit and BuiltAt are empty when unknown.
type Info struct {
	Version string
	Commit  string
	BuiltAt string
}

// Current returns the release values when set, otherwise what the Go
// toolchain recorded: the module version for `go install ...@v0.1.0`, or a
// pseudo-version and the commit for a build from a checkout.
func Current() Info {
	info := Info{Version: version, Commit: commit, BuiltAt: builtAt}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return withDefaults(info)
	}
	if info.Version == "" && bi.Main.Version != "(devel)" {
		info.Version = strings.TrimPrefix(bi.Main.Version, "v")
	}
	for _, s := range bi.Settings {
		switch {
		case s.Key == "vcs.revision" && info.Commit == "":
			info.Commit = s.Value
		case s.Key == "vcs.time" && info.BuiltAt == "":
			info.BuiltAt = s.Value
		}
	}
	return withDefaults(info)
}

func withDefaults(info Info) Info {
	if info.Version == "" {
		info.Version = "dev"
	}
	return info
}
