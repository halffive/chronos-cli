//go:build !unix

package subprocess

import "os"

func exitCode(state *os.ProcessState) int {
	return state.ExitCode()
}
