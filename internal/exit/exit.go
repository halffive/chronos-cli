// Package exit maps failures to the CLI's documented exit codes and to the
// error object printed with --json. The codes and error codes are part of the
// CLI's stable contract: add new ones, never renumber or rename.
package exit

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Code int

const (
	OK              Code = 0
	Failure         Code = 1
	Usage           Code = 2
	NotFound        Code = 3
	Auth            Code = 4
	Rejected        Code = 5
	Retryable       Code = 6
	OutcomeUnknown  Code = 7
	JobUnsuccessful Code = 8
	WaitTimeout     Code = 9
	Interrupted     Code = 130
)

// Error codes the CLI produces itself. API failures carry the API's own code.
const (
	CodeError           = "error"
	CodeUsage           = "usage_error"
	CodeAPIKeyMissing   = "api_key_missing"
	CodeNetwork         = "network_error"
	CodeOutcomeUnknown  = "outcome_unknown"
	CodeJobUnsuccessful = "job_unsuccessful"
	CodeWaitTimeout     = "wait_timeout"
	CodeWaitFailed      = "wait_failed"
	CodeInterrupted     = "interrupted"
	CodeCancelled       = "cancelled"
)

type Description struct {
	Code    Code
	Meaning string
}

// Codes lists every exit code in order, for the exit-codes help topic.
func Codes() []Description {
	return []Description{
		{OK, "Success, including --dry-run."},
		{Failure, "Any other failure."},
		{Usage, "Bad flags or arguments, or a confirmation that needs --yes."},
		{NotFound, "The resource does not exist."},
		{Auth, "No usable API key, or the API rejected it."},
		{Rejected, "The API rejected the request for validation, a conflict or a plan limit. Read error.code."},
		{Retryable, "Safe to run again: rate limited, a read that failed, or a write that never left this machine."},
		{OutcomeUnknown, "A write failed after it may have reached the API. Check its state before running the command again."},
		{JobUnsuccessful, "The job you waited for failed, was cancelled or was skipped."},
		{WaitTimeout, "The wait ended before the job finished. Continue with chronos jobs wait <id>."},
		{Interrupted, "Interrupted by Ctrl-C or SIGTERM."},
	}
}

// Error is a failure the CLI decides on itself, such as a usage error.
type Error struct {
	Exit    Code
	Code    string
	Message string
	// Retryable marks a failure that is safe to repeat although its exit
	// code is not Retryable, such as a wait that timed out on its own.
	Retryable bool
	// Cause is the API failure behind this one, whose status and request id
	// are reported with it.
	Cause error
}

func (e *Error) Error() string { return e.Message }

func (e *Error) Unwrap() error { return e.Cause }

func Failuref(format string, args ...any) *Error {
	return &Error{Exit: Failure, Code: CodeError, Message: fmt.Sprintf(format, args...)}
}

func Usagef(format string, args ...any) *Error {
	return &Error{Exit: Usage, Code: CodeUsage, Message: fmt.Sprintf(format, args...)}
}

func MissingKeyf(format string, args ...any) *Error {
	return &Error{Exit: Auth, Code: CodeAPIKeyMissing, Message: fmt.Sprintf(format, args...)}
}

// Sentence turns a library's error text, such as cobra's "unknown flag:
// --x", into a sentence that matches the CLI's own messages.
func Sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	s = string(unicode.ToUpper(r)) + s[size:]
	if !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "?") {
		s += "."
	}
	return s
}
