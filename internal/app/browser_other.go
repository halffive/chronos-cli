//go:build !unix || darwin

package app

import "os/exec"

func detach(*exec.Cmd) {}
