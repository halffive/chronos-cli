package exit_test

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"

	chronos "chronos.sh/go"

	"chronos.sh/cli/internal/exit"
)

func networkError(err error) error {
	return &chronos.NetworkError{Err: &url.Error{Op: "Post", URL: "https://api.chronos.sh/v1/jobs", Err: err}}
}

func TestClassifyNetworkErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		method string
		want   exit.Code
	}{
		{"DNS failure on a write never left", networkError(&net.DNSError{Err: "no such host", Name: "api.chronos.sh"}), "POST", exit.Retryable},
		{"refused connect on a write never left", networkError(&net.OpError{Op: "dial", Err: errors.New("connection refused")}), "POST", exit.Retryable},
		{"reset during a write may have landed", networkError(&net.OpError{Op: "read", Err: errors.New("connection reset")}), "POST", exit.OutcomeUnknown},
		{"reset during a read is safe", networkError(&net.OpError{Op: "read", Err: errors.New("connection reset")}), "GET", exit.Retryable},
		{"failure before any request", networkError(errors.New("boom")), "", exit.Retryable},
		{"proxy down on a write never left", networkError(&net.OpError{Op: "proxyconnect", Err: errors.New("connection refused")}), "POST", exit.Retryable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exit.Classify(tt.err, exit.Sent{LastMethod: tt.method})
			if got.Exit != tt.want || got.Retryable != (tt.want == exit.Retryable) {
				t.Errorf("Classify = exit %d retryable %v, want %d", got.Exit, got.Retryable, tt.want)
			}
		})
	}
}

func TestClassifyCLIErrors(t *testing.T) {
	wrapped := fmt.Errorf("context: %w", exit.Usagef("bad flag"))
	if got := exit.Classify(wrapped, exit.Sent{}); got.Exit != exit.Usage || got.Code != exit.CodeUsage || got.Message != "bad flag" {
		t.Errorf("Classify(usage) = %+v", got)
	}
	if got := exit.Classify(errors.New("disk full"), exit.Sent{}); got.Exit != exit.Failure || got.Code != exit.CodeError {
		t.Errorf("Classify(other) = %+v", got)
	}
}

func TestAppliedWriteIsNeverSafeToRepeat(t *testing.T) {
	readFailed := &chronos.APIError{StatusCode: 503, Code: "error", Message: "Service unavailable"}
	got := exit.Classify(readFailed, exit.Sent{LastMethod: "GET", WriteApplied: true})
	if got.Exit != exit.OutcomeUnknown || got.Retryable {
		t.Errorf("Classify = exit %d retryable %v, want %d and not retryable", got.Exit, got.Retryable, exit.OutcomeUnknown)
	}
	timedOut := &exit.Error{Exit: exit.WaitTimeout, Code: exit.CodeWaitTimeout, Retryable: true}
	if got := exit.Classify(timedOut, exit.Sent{WriteApplied: true}); got.Exit != exit.WaitTimeout || got.Retryable {
		t.Errorf("wait timeout after a write = %+v", got)
	}
}

func TestRedirectOnAReadIsNotRetryable(t *testing.T) {
	moved := &chronos.APIError{StatusCode: 308, Message: "Permanent Redirect"}
	if got := exit.Classify(moved, exit.Sent{LastMethod: "GET"}); got.Exit != exit.Failure || got.Retryable {
		t.Errorf("Classify = %+v", got)
	}
}
