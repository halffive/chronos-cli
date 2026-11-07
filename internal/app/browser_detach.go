//go:build unix && !darwin

package app

import (
	"os/exec"
	"syscall"
)

// detach starts the opener in its own process group. The browser xdg-open
// starts would otherwise share the terminal's foreground group, and Ctrl-C
// in chronos would close it.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
