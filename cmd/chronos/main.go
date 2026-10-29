// Command chronos manages Chronos schedules, jobs and executions.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmd/root"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// After the first Ctrl-C, restore the default so a second one kills the process.
	go func() {
		<-ctx.Done()
		stop()
	}()
	code := root.Run(ctx, app.NewSystemFactory(), os.Args[1:])
	stop()
	os.Exit(int(code))
}
