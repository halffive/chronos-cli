//go:build unix

package monitors_test

import (
	"os"
	"os/exec"
	"testing"

	"chronos.sh/cli/internal/clitest"
)

func TestExecRunsTheCommandWhenStderrIsAClosedPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	defer w.Close()
	args := append([]string{systemFlag, "monitors", "exec", "--"}, clitest.ChildCommand("out:dumping database app", "exit:5")...)
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = []string{"CHRONOS_CONFIG_DIR=" + t.TempDir()}
	cmd.Stderr = w
	out, _ := cmd.Output()
	if code := cmd.ProcessState.ExitCode(); code != 5 || string(out) != "dumping database app\n" {
		t.Errorf("exit = %d (%v), stdout = %q; want the command's exit code 5 and its output", code, cmd.ProcessState, out)
	}
}
