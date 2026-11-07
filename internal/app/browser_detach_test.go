//go:build unix && !darwin

package app_test

import (
	"os/exec"
	"syscall"
	"testing"

	"chronos.sh/cli/internal/app"
)

func TestDetachedCommandsGetTheirOwnProcessGroup(t *testing.T) {
	cmd := exec.Command("sleep", "5")
	app.Detach(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if pgid == syscall.Getpgrp() {
		t.Errorf("the opener shares chronos's process group %d, so Ctrl-C would reach the browser", pgid)
	}
}
