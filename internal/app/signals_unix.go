//go:build unix

package app

import (
	"os"
	"syscall"
)

// ForwardSignals are the signals a command that runs another catches:
// InterruptSignals, SIGQUIT, and SIGPIPE so that writing to a closed pipe
// fails instead of killing chronos.
func ForwardSignals() []os.Signal {
	return append(InterruptSignals(), syscall.SIGQUIT, syscall.SIGPIPE)
}

func hasControllingTerminal() bool {
	fd, err := syscall.Open("/dev/tty", syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return false
	}
	syscall.Close(fd)
	return true
}
