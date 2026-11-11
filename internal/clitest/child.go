package clitest

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const childFlag = "-chronos-test-child"

// ChildCommand returns the command line that runs the test binary as a child
// command, once the package's TestMain calls ServeChild. The child runs steps
// in order:
//
//	out:<text>   write text and a newline to stdout
//	err:<text>   the same on stderr
//	bytes:<n>    write n bytes to stdout, in lines of 64
//	stdin        copy stdin to stdout
//	sleep:<d>    sleep for the duration d
//	spew         write to stdout until a write fails, for at most 10s
//	hold-stdout  leave a process running that holds stdout open for 3s
//	exit:<n>     exit with code n
//
// Any other step is written to stdout as "arg <step>".
func ChildCommand(steps ...string) []string {
	return append([]string{os.Args[0], childFlag}, steps...)
}

// ServeChild runs the child and exits when the test binary was started by
// ChildCommand, and returns otherwise.
func ServeChild() {
	if len(os.Args) < 2 || os.Args[1] != childFlag {
		return
	}
	for _, step := range os.Args[2:] {
		runChildStep(step)
	}
	os.Exit(0)
}

func runChildStep(step string) {
	name, value, _ := strings.Cut(step, ":")
	switch name {
	case "out":
		fmt.Println(value)
	case "err":
		fmt.Fprintln(os.Stderr, value)
	case "bytes":
		n, _ := strconv.Atoi(value)
		line := append(bytes.Repeat([]byte("x"), 63), '\n')
		os.Stdout.Write(bytes.Repeat(line, n/len(line)+1)[:n])
	case "stdin":
		io.Copy(os.Stdout, os.Stdin)
	case "sleep":
		d, _ := time.ParseDuration(value)
		time.Sleep(d)
	case "spew":
		for end := time.Now().Add(10 * time.Second); time.Now().Before(end); {
			if _, err := fmt.Println("output nobody reads"); err != nil {
				return
			}
		}
	case "hold-stdout":
		holder := exec.Command(os.Args[0], childFlag, "sleep:3s")
		holder.Stdout = os.Stdout
		holder.Start()
	case "exit":
		code, _ := strconv.Atoi(value)
		os.Exit(code)
	default:
		fmt.Println("arg", step)
	}
}
