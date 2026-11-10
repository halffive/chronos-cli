package monitors

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/output"
	"chronos.sh/cli/internal/ping"
	"chronos.sh/cli/internal/subprocess"
)

const pingURLEnv = "CHRONOS_PING_URL"

type execOptions struct {
	pingURL    string
	noPingBody bool
}

func newExecCmd(f *app.Factory) *cobra.Command {
	var opts execOptions
	spec := cmdutil.CommandSpec{
		Use:   "exec [flags] [--] <command> [args...]",
		Short: "Run a command and send its start, exit code and output to a monitor",
		Long: "Run a command and report it to a heartbeat monitor. chronos sends a start ping, runs the command, then sends its exit code and the last 10 KB of its output, and exits with the command's exit code. The command keeps your stdin. Its stdout and stderr pass through chronos to yours, so the command sees a pipe, not a terminal. With --no-ping-body, or without a ping URL, the command gets your stdout and stderr directly.\n\n" +
			"A ping that fails, or a missing ping URL, prints a line on stderr and never stops the command or changes its exit code.\n\n" +
			"Flags after the command belong to the command.",
		Example: `  CHRONOS_PING_URL=https://ping.chronos.sh/<token> chronos monitors exec -- /usr/local/bin/backup.sh
  chronos monitors exec --ping-url https://ping.chronos.sh/<token> -- /usr/local/bin/backup.sh
  chronos monitors exec --no-ping-body pg_dump -f /backups/app.sql app`,
		Args: commandArgs,
	}
	cmd := cmdutil.NewCommand(f, spec, func(ctx context.Context, _ *cobra.Command, args []string) (output.View, error) {
		return nil, runExec(ctx, f, opts, args)
	})
	flags := cmd.Flags()
	flags.SetInterspersed(false)
	flags.StringVar(&opts.pingURL, "ping-url", "", "The monitor's ping URL; defaults to "+pingURLEnv)
	flags.BoolVar(&opts.noPingBody, "no-ping-body", false, "Keep the command's output out of the pings")
	return cmd
}

func commandArgs(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		return nil
	}
	return exit.Usagef("No command to run. Put it after the flags, such as chronos monitors exec -- ./backup.sh.")
}

type execution struct {
	f      *app.Factory
	name   string
	errOut io.Writer
	pings  *ping.Client
	// pingURL is nil when this run sends no pings.
	pingURL *ping.URL
	// tail is nil when the pings carry no output.
	tail *ping.Tail
}

func runExec(ctx context.Context, f *app.Factory, opts execOptions, args []string) error {
	caught := make(chan os.Signal, 8)
	signal.Notify(caught, app.ForwardSignals()...)
	defer signal.Stop(caught)
	finalCtx, interrupt := context.WithCancel(context.WithoutCancel(ctx))
	defer interrupt()
	relay := startSignalRelay(signalRelayOptions{caught: caught, controllingTerminal: f.IO.HasControllingTerminal, cancelResult: interrupt})
	defer relay.stop()

	e := &execution{f: f, name: commandName(args[0]), errOut: lockWrites(f.IO.ErrOut), pings: f.PingClient()}
	e.pingURL = e.resolvePingURL(opts.pingURL)
	if e.pingURL != nil && !opts.noPingBody {
		e.tail = ping.NewTail(ping.BodyMaxBytes)
	}

	start := e.startPing(ctx)
	stdout, stderr := e.streams()
	res := subprocess.Run(subprocess.Options{
		Name: args[0], Args: args[1:],
		Stdin: f.IO.In, Stdout: stdout, Stderr: stderr,
		Signals: relay.forward,
		Started: start.send,
	})
	relay.commandExited()
	startErr := start.finish()

	var body []byte
	if res.StartErr != nil {
		reason := fmt.Sprintf("%s: %v", e.name, res.StartErr)
		e.say("%s", reason)
		body = []byte("chronos: " + reason)
	} else if e.tail != nil {
		body = e.tail.Bytes()
	}
	if e.pingURL != nil && !ping.IsNotFound(startErr) {
		e.sendResult(finalCtx, res, body)
	}
	if res.ExitCode == 0 {
		return nil
	}
	return &exit.ChildExit{Code: exit.Code(res.ExitCode)}
}

func commandName(arg string) string {
	if strings.Contains(arg, "://") {
		return ping.MaskURL(arg)
	}
	return arg
}

func (e *execution) say(format string, args ...any) {
	fmt.Fprintln(e.errOut, output.Clean("chronos: "+fmt.Sprintf(format, args...)))
}

func (e *execution) resolvePingURL(flag string) *ping.URL {
	raw := strings.TrimSpace(flag)
	if raw == "" {
		raw = strings.TrimSpace(e.f.Getenv(pingURLEnv))
	}
	if raw == "" {
		e.say("no ping URL, so this run sends no pings. Set %s in the crontab or pass --ping-url.", pingURLEnv)
		return nil
	}
	u, err := ping.ParseURL(raw)
	switch {
	case err == nil:
		return u
	case errors.Is(err, ping.ErrMaskedToken):
		e.say("%s is a masked ping URL, so this run sends no pings. Read the full URL with chronos monitors get <id>, using a login or API key with monitors:write.", ping.MaskURL(raw))
	case errors.Is(err, ping.ErrInsecure):
		e.say("%s must use https, so this run sends no pings. Plain http is allowed only for localhost.", ping.MaskURL(raw))
	default:
		e.say("%s is not a ping URL, so this run sends no pings. Use the URL from chronos monitors get <id>, which ends with the token.", ping.MaskURL(raw))
	}
	return nil
}

// streams hands the command the CLI's own streams when there is no tail, so a
// terminal stays a terminal.
func (e *execution) streams() (stdout, stderr io.Writer) {
	if e.tail == nil {
		return e.f.IO.Out, e.errOut
	}
	return io.MultiWriter(e.tail, e.f.IO.Out), io.MultiWriter(e.tail, e.errOut)
}

// startPing sends the start ping while the command runs, so a slow network
// never delays the command.
type startPing struct {
	send   func()
	finish func() error
}

func (e *execution) startPing(ctx context.Context) startPing {
	if e.pingURL == nil {
		return startPing{send: func() {}, finish: func() error { return nil }}
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	var wg sync.WaitGroup
	var err error
	send := func() {
		wg.Go(func() {
			err = e.pings.Start(ctx, e.pingURL)
			if err != nil && ctx.Err() == nil {
				e.warnStart(err)
			}
		})
	}
	// finish runs after the command has exited, so it cancels a start ping
	// still in flight rather than let it delay the result ping.
	finish := func() error {
		cancel()
		wg.Wait()
		return err
	}
	return startPing{send: send, finish: finish}
}

func (e *execution) warnStart(err error) {
	if ping.IsNotFound(err) {
		e.say("%s answered 404 to the start ping, so this run sends no more pings. The URL is wrong or was rotated. Run chronos monitors get <id> for the current one.", e.pingURL.Masked())
		return
	}
	e.say("could not send the start ping to %s (%v). The exit code is still sent when the command ends.", e.pingURL.Masked(), err)
}

func (e *execution) sendResult(ctx context.Context, res subprocess.Result, body []byte) {
	code := res.ExitCode
	result := fmt.Sprintf("exit code %d", code)
	var err error
	if code >= 0 && code <= 255 {
		err = e.pings.ExitCode(ctx, e.pingURL, code, body)
	} else {
		result = "a fail ping for " + result
		err = e.pings.Fail(ctx, e.pingURL, body)
	}
	masked := e.pingURL.Masked()
	switch {
	case errors.Is(err, context.Canceled):
		e.say("did not send %s to %s, because you interrupted it.", result, masked)
	case ping.IsNotFound(err):
		e.say("could not send %s to %s (it answered 404), so the monitor did not get this run's result. The URL is wrong or was rotated. Run chronos monitors get <id> for the current one.", result, masked)
	case err != nil:
		e.say("could not send %s to %s (%v), so the monitor did not get this run's result.", result, masked, err)
	case e.f.IO.ErrTTY && !e.f.JSON:
		e.confirm(res, result, masked)
	}
}

func (e *execution) confirm(res subprocess.Result, result, masked string) {
	written := int64(0)
	if e.tail != nil {
		written = e.tail.Written()
	}
	switch {
	case res.StartErr != nil:
		e.say("sent %s to %s with this error.", result, masked)
	case e.tail == nil:
		e.say("sent %s to %s, without the output.", result, masked)
	case written == 0:
		e.say("sent %s and no output to %s.", result, masked)
	case written <= ping.BodyMaxBytes:
		e.say("sent %s and %s of output to %s.", result, ping.FormatSize(written), masked)
	default:
		e.say("sent %s and the last %s of %s of output to %s.", result, ping.FormatSize(ping.BodyMaxBytes), ping.FormatSize(written), masked)
	}
}

type signalRelayOptions struct {
	caught              <-chan os.Signal
	controllingTerminal bool
	cancelResult        func()
}

// signalRelay passes caught signals to the command while it runs. Once the
// command has exited, a signal cancels the result ping instead, so one more
// Ctrl-C always ends chronos.
type signalRelay struct {
	forward chan os.Signal
	exited  chan struct{}
	done    chan struct{}
	wg      sync.WaitGroup
}

func startSignalRelay(o signalRelayOptions) *signalRelay {
	r := &signalRelay{forward: make(chan os.Signal, 8), exited: make(chan struct{}), done: make(chan struct{})}
	r.wg.Go(func() {
		if r.whileRunning(o) {
			r.afterExit(o)
		}
	})
	return r
}

func (r *signalRelay) commandExited() { close(r.exited) }

func (r *signalRelay) stop() {
	close(r.done)
	r.wg.Wait()
}

// whileRunning reports whether the command exited before the relay stopped.
func (r *signalRelay) whileRunning(o signalRelayOptions) bool {
	for {
		select {
		case sig := <-o.caught:
			if forwards(sig, o.controllingTerminal) {
				r.send(sig)
			}
		case <-r.exited:
			return true
		case <-r.done:
			return false
		}
	}
}

// send never blocks, since the command may exit before it reads the signal.
func (r *signalRelay) send(sig os.Signal) {
	select {
	case r.forward <- sig:
	default:
	}
}

func (r *signalRelay) afterExit(o signalRelayOptions) {
	// Signals already queued arrived while the command ran, such as the
	// Ctrl-C that ended it.
	for len(o.caught) > 0 {
		<-o.caught
	}
	for {
		select {
		case sig := <-o.caught:
			if sig != syscall.SIGPIPE {
				o.cancelResult()
				return
			}
		case <-r.done:
			return
		}
	}
}

// forwards leaves SIGINT, SIGQUIT and SIGHUP to a controlling terminal, which
// already sends Ctrl-C, Ctrl-\ and a hang-up to the command; a second copy
// would read as a forced quit.
func forwards(sig os.Signal, controllingTerminal bool) bool {
	switch sig {
	case syscall.SIGTERM:
		return true
	case syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP:
		return !controllingTerminal
	}
	return false
}

// lockWrites serializes writes to w, which the command's stderr and
// chronos's own lines share. A file needs no lock, and the command gets it
// directly.
func lockWrites(w io.Writer) io.Writer {
	if _, ok := w.(*os.File); ok {
		return w
	}
	return &lockedWriter{w: w}
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *lockedWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
