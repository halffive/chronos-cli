// Package transport sits between the SDK and the network. It records what
// was sent, which decides whether a failure is safe to repeat, and captures
// requests instead of sending them for --dry-run.
package transport

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
)

type Recorder struct {
	// Base sends the request. Nil means http.DefaultTransport.
	Base http.RoundTripper
	// DryRun captures every request and fails it with *Captured instead of
	// sending it.
	DryRun bool

	mu      sync.Mutex
	method  string
	applied bool
}

// Captured is the error a dry run returns in place of a response. Body is
// nil when the request has none.
type Captured struct {
	Method string          `json:"method"`
	URL    string          `json:"url"`
	Body   json.RawMessage `json:"body"`
}

func (c *Captured) Error() string { return "dry run: request not sent" }

// LastMethod returns the method of the last request, or "" if none was made.
func (r *Recorder) LastMethod() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.method
}

// WriteApplied reports whether a request other than a GET got a 2xx answer,
// so the API has acted on it.
func (r *Recorder) WriteApplied() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.applied
}

func (r *Recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.method = req.Method
	r.mu.Unlock()

	if r.DryRun {
		return nil, capture(req)
	}
	res, err := r.base().RoundTrip(req)
	if err == nil && req.Method != http.MethodGet && res.StatusCode < 300 {
		r.mu.Lock()
		r.applied = true
		r.mu.Unlock()
	}
	return res, err
}

func capture(req *http.Request) error {
	captured := &Captured{Method: req.Method, URL: req.URL.String()}
	if req.Body == nil {
		return captured
	}
	defer req.Body.Close()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(body)) > 0 {
		captured.Body = body
	}
	return captured
}

func (r *Recorder) base() http.RoundTripper {
	if r.Base == nil {
		return http.DefaultTransport
	}
	return r.Base
}
