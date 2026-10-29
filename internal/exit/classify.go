package exit

import (
	"context"
	"errors"
	"net"
	"net/http"

	chronos "chronos.sh/go"
)

// Outcome is a classified failure: the exit code, plus the error object
// printed with --json.
type Outcome struct {
	Exit        Code         `json:"-"`
	Status      *int         `json:"status"`
	Code        string       `json:"code"`
	Message     string       `json:"message"`
	RequestID   *string      `json:"request_id"`
	FieldErrors []FieldError `json:"field_errors"`
	// Retryable reports that running the same command again is safe.
	Retryable bool `json:"retryable"`
}

// FieldError is one rejected field. Field is the API's own string: a dotted
// path, comma-joined names for a rule across fields, or null.
type FieldError struct {
	Field   *string `json:"field"`
	Message string  `json:"message"`
	Value   any     `json:"value"`
}

const outcomeUnknownSuffix = ". The request may have reached the API, so check its state before running the command again."

// Sent is what the run sent before it failed.
type Sent struct {
	// LastMethod is the HTTP method of the last request, or empty when none
	// was sent.
	LastMethod string
	// WriteApplied is true when a write got a 2xx earlier in the run, so
	// running the command again would repeat it.
	WriteApplied bool
}

// Classify maps err to an Outcome. After an applied write, a retryable
// failure becomes outcome_unknown, since running the command again would
// repeat the write.
func Classify(err error, sent Sent) Outcome {
	o := classify(err, sent.LastMethod)
	if !sent.WriteApplied {
		return o
	}
	o.Retryable = false
	if o.Exit == Retryable {
		o.Exit, o.Code, o.Message = OutcomeUnknown, CodeOutcomeUnknown, o.Message+writeAppliedSuffix
	}
	return o
}

const writeAppliedSuffix = ". An earlier request in this command was applied, so check its state before running the command again."

func classify(err error, method string) Outcome {
	if e, ok := errors.AsType[*Error](err); ok {
		o := newOutcome(e.Exit, e.Code, e.Message)
		o.Retryable = o.Retryable || e.Retryable
		if apiErr, ok := errors.AsType[*chronos.APIError](e.Cause); ok {
			o.Status, o.RequestID = statusAndRequestID(apiErr)
		}
		return o
	}
	if errors.Is(err, context.Canceled) {
		if isRead(method) {
			return newOutcome(Interrupted, CodeInterrupted, "Interrupted.")
		}
		return newOutcome(Interrupted, CodeInterrupted, "Interrupted"+outcomeUnknownSuffix)
	}
	if apiErr, ok := errors.AsType[*chronos.APIError](err); ok {
		return classifyAPI(apiErr, method)
	}
	if netErr, ok := errors.AsType[*chronos.NetworkError](err); ok {
		return classifyNetwork(netErr, method)
	}
	if errors.Is(err, chronos.ErrInvalidResponse) && !isRead(method) {
		return newOutcome(OutcomeUnknown, CodeOutcomeUnknown, err.Error()+outcomeUnknownSuffix)
	}
	if errors.Is(err, chronos.ErrInvalidID) {
		return newOutcome(Usage, CodeUsage, err.Error())
	}
	return newOutcome(Failure, CodeError, err.Error())
}

func classifyAPI(apiErr *chronos.APIError, method string) Outcome {
	code := string(apiErr.Code)
	if code == "" {
		code = CodeError
	}
	message := apiErr.Message
	if message == "" {
		message = http.StatusText(apiErr.StatusCode)
	}
	o := newOutcome(apiExit(apiErr.StatusCode, method), code, message)
	if o.Exit == OutcomeUnknown {
		o.Code, o.Message = CodeOutcomeUnknown, message+outcomeUnknownSuffix
	}
	o.Status, o.RequestID = statusAndRequestID(apiErr)
	for _, fe := range apiErr.FieldErrors {
		o.FieldErrors = append(o.FieldErrors, newFieldError(fe))
	}
	return o
}

func apiExit(status int, method string) Code {
	switch {
	case status == http.StatusUnauthorized:
		return Auth
	case status == http.StatusNotFound:
		return NotFound
	case status == http.StatusTooManyRequests:
		return Retryable
	case status >= 400 && status < 500:
		return Rejected
	case status < 400 && isRead(method):
		return Failure
	case isRead(method):
		return Retryable
	}
	// A 5xx, or a 2xx or 3xx the SDK could not use, after a write means the
	// write may have been applied.
	return OutcomeUnknown
}

func classifyNetwork(netErr *chronos.NetworkError, method string) Outcome {
	if isRead(method) || neverSent(netErr) {
		return newOutcome(Retryable, CodeNetwork, netErr.Error())
	}
	return newOutcome(OutcomeUnknown, CodeOutcomeUnknown, netErr.Error()+outcomeUnknownSuffix)
}

// neverSent reports a failure before a connection to the API existed, so no
// byte of the request reached it.
func neverSent(err error) bool {
	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return true
	}
	opErr, ok := errors.AsType[*net.OpError](err)
	return ok && (opErr.Op == "dial" || opErr.Op == "proxyconnect")
}

func isRead(method string) bool {
	return method == "" || method == http.MethodGet
}

func newOutcome(exit Code, code, message string) Outcome {
	return Outcome{Exit: exit, Code: code, Message: message, FieldErrors: []FieldError{}, Retryable: exit == Retryable}
}

func statusAndRequestID(apiErr *chronos.APIError) (*int, *string) {
	status := apiErr.StatusCode
	if apiErr.RequestID == "" {
		return &status, nil
	}
	requestID := apiErr.RequestID
	return &status, &requestID
}

func newFieldError(fe chronos.FieldError) FieldError {
	out := FieldError{Message: fe.Message, Value: fe.Value}
	if fe.Field != "" {
		out.Field = &fe.Field
	}
	return out
}
