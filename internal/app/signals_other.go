//go:build !unix

package app

import "os"

func ForwardSignals() []os.Signal {
	return InterruptSignals()
}

func hasControllingTerminal() bool { return false }
