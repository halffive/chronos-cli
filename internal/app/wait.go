package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	chronos "chronos.sh/go"

	"chronos.sh/cli/internal/exit"
)

const DefaultWaitTimeout = 60 * time.Second

type WaitOptions struct {
	JobID   string
	Timeout time.Duration
	// Job is the latest state already known, printed if the wait learns
	// nothing newer.
	Job *chronos.Job
}

// WaitForJob waits until the job finishes. It returns the job with a nil
// error when it completed, and with an *exit.Error when it finished any other
// way or the wait ended first.
//
// When a write earlier in the run was applied, a failed check exits 9
// instead of 6 or 7, since running the command again would repeat the write.
func (f *Factory) WaitForJob(ctx context.Context, c *chronos.Client, opts WaitOptions) (*chronos.Job, error) {
	if opts.Timeout <= 0 {
		return opts.Job, errors.New("chronos: WaitForJob needs a positive timeout; check the flag with CheckWaitTimeout before the write")
	}
	afterWrite := f.Sent().WriteApplied
	waitCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	job, err := c.Jobs.Wait(waitCtx, opts.JobID, chronos.JobWaitOptions{PollInterval: f.JobPollInterval})
	if err == nil {
		return job, unlessCompleted(job)
	}
	if ctx.Err() != nil {
		return opts.Job, ctx.Err()
	}
	if timeout, ok := errors.AsType[*chronos.WaitTimeoutError](err); ok {
		last := opts.Job
		if timeout.Job != nil {
			last = timeout.Job
		}
		return last, &exit.Error{
			Exit:      exit.WaitTimeout,
			Code:      exit.CodeWaitTimeout,
			Message:   fmt.Sprintf("Job %s did not finish within %ds. Keep waiting with chronos jobs wait %s.", opts.JobID, int(opts.Timeout.Seconds()), opts.JobID),
			Retryable: true,
		}
	}
	if afterWrite {
		return opts.Job, &exit.Error{
			Exit:    exit.WaitTimeout,
			Code:    exit.CodeWaitFailed,
			Message: fmt.Sprintf("The API accepted the request, but checking on job %s failed: %v. Keep waiting with chronos jobs wait %s.", opts.JobID, err, opts.JobID),
			Cause:   err,
		}
	}
	return nil, err
}

// CheckWaitTimeout rejects a --wait-timeout below one second. Call it before
// the write a wait follows, so a bad flag never leaves the write applied.
func CheckWaitTimeout(timeout time.Duration) error {
	if timeout < time.Second {
		return exit.Usagef("--wait-timeout must be at least 1s.")
	}
	return nil
}

func unlessCompleted(job *chronos.Job) error {
	if job.Status == chronos.JobStatusCompleted {
		return nil
	}
	return &exit.Error{
		Exit:    exit.JobUnsuccessful,
		Code:    exit.CodeJobUnsuccessful,
		Message: fmt.Sprintf("Job %s finished with status %s.", job.ID, job.Status),
	}
}
