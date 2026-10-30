// Package clitest runs chronos commands against a fake API in tests. Use it
// from external test packages (package schedules_test), since it imports the
// whole command tree.
package clitest

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"flag"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmd/root"
	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/exit"
)

const APIKey = "chrns_testkey0123456789abcdefghijklmnop"

// Now is the fixed time tests run at unless Options.Now is set.
func Now() time.Time {
	return time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
}

var update = flag.Bool("update", false, "rewrite golden files")

//go:embed testdata/*.json
var fixtures embed.FS

// Fixture returns testdata/<name>.json: an API object as the API returns it.
func Fixture(name string) string {
	b, err := fixtures.ReadFile("testdata/" + name + ".json")
	if err != nil {
		panic(err)
	}
	return string(b)
}

type Response struct {
	Status int
	Header map[string]string
	Body   string
	// Delay holds the response back, to make a request time out.
	Delay time.Duration
}

// OK wraps data in the API's success envelope.
func OK(data string) Response {
	return Response{Status: http.StatusOK, Body: `{"success":true,"message":"ok","data":` + data + `}`}
}

// Page wraps data in the API's list envelope.
func Page(data string, paging chronos.Paging) Response {
	meta, _ := json.Marshal(map[string]chronos.Paging{"paging": paging})
	return Response{Status: http.StatusOK, Body: `{"success":true,"message":"ok","data":` + data + `,"meta":` + string(meta) + `}`}
}

// Failure is an API error response. FieldErrors is a JSON array, or empty
// for none.
type Failure struct {
	Status      int
	Code        string
	Message     string
	FieldErrors string
}

// Response returns the error envelope the API sends for f.
func (f Failure) Response() Response {
	fieldErrors := f.FieldErrors
	if fieldErrors == "" {
		fieldErrors = "[]"
	}
	message, _ := json.Marshal(f.Message)
	return Response{
		Status: f.Status,
		Header: map[string]string{"X-Request-Id": "req_test"},
		Body:   `{"success":false,"message":` + string(message) + `,"code":"` + f.Code + `","errors":` + fieldErrors + `}`,
	}
}

type Request struct {
	Method     string
	RequestURI string
	Header     http.Header
	Body       string
}

// API replays responses in order, repeating the last one, and records every
// request. A request to an API with no responses fails the test.
type API struct {
	URL string

	t         *testing.T
	mu        sync.Mutex
	responses []Response
	requests  []Request
}

func NewAPI(t *testing.T, responses ...Response) *API {
	t.Helper()
	api := &API{t: t, responses: responses}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	api.URL = server.URL
	return api
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	a.mu.Lock()
	a.requests = append(a.requests, Request{Method: r.Method, RequestURI: r.RequestURI, Header: r.Header.Clone(), Body: string(body)})
	if len(a.responses) == 0 {
		a.mu.Unlock()
		a.t.Errorf("unexpected request %s %s", r.Method, r.RequestURI)
		w.WriteHeader(http.StatusTeapot)
		return
	}
	res := a.responses[min(len(a.requests), len(a.responses))-1]
	a.mu.Unlock()

	if res.Delay > 0 {
		select {
		case <-time.After(res.Delay):
		case <-r.Context().Done():
			return
		}
	}
	for k, v := range res.Header {
		w.Header().Set(k, v)
	}
	w.WriteHeader(res.Status)
	io.WriteString(w, res.Body)
}

func (a *API) Requests() []Request {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Request(nil), a.requests...)
}

// Env points the CLI at this API with the test key.
func (a *API) Env() map[string]string {
	return map[string]string{"CHRONOS_API_KEY": APIKey, "CHRONOS_BASE_URL": a.URL}
}

type Options struct {
	Args []string
	// Env is the whole environment. CHRONOS_CONFIG_DIR defaults to a fresh
	// temporary directory.
	Env       map[string]string
	Stdin     string
	StdinTTY  bool
	StdoutTTY bool
	Width     int
	// Secret is what a hidden prompt reads.
	Secret string
	Now    time.Time
	// Context defaults to the test's context.
	Context context.Context
	// ClientOptions are appended to the defaults, which turn off request
	// retries so failures surface at once.
	ClientOptions []chronos.Option
	// Extend adds test-only commands to the root command.
	Extend func(f *app.Factory, root *cobra.Command)
	// Keychain defaults to an empty in-memory keychain, so tests never touch
	// the system one.
	Keychain config.Keychain
}

type Result struct {
	Stdout string
	Stderr string
	Code   exit.Code
}

func Run(t *testing.T, opts Options) Result {
	t.Helper()
	env := map[string]string{"CHRONOS_CONFIG_DIR": t.TempDir()}
	maps.Copy(env, opts.Env)
	now := opts.Now
	if now.IsZero() {
		now = Now()
	}
	ctx := opts.Context
	if ctx == nil {
		ctx = t.Context()
	}
	var stdout, stderr bytes.Buffer
	f := &app.Factory{
		IO: &app.IOStreams{
			In:     strings.NewReader(opts.Stdin),
			Out:    &stdout,
			ErrOut: &stderr,
			InTTY:  opts.StdinTTY,
			OutTTY: opts.StdoutTTY,
			Width:  opts.Width,
			ReadSecret: func(context.Context) (string, error) {
				return opts.Secret, nil
			},
		},
		Getenv:          func(k string) string { return env[k] },
		Now:             func() time.Time { return now },
		ClientOptions:   append([]chronos.Option{chronos.WithMaxRequestRetries(0)}, opts.ClientOptions...),
		JobPollInterval: time.Millisecond,
		Keychain:        opts.Keychain,
	}
	if f.Keychain == nil {
		f.Keychain = &Keychain{}
	}
	cmd := root.NewCmd(f)
	if opts.Extend != nil {
		opts.Extend(f, cmd)
	}
	code := root.Execute(ctx, f, cmd, opts.Args)
	return Result{Stdout: stdout.String(), Stderr: stderr.String(), Code: code}
}

// Golden compares got with testdata/<name>.golden in the calling package.
// Run go test with -update to rewrite it.
func Golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file (run go test -update to create it): %v", err)
	}
	if got != string(want) {
		t.Errorf("%s does not match.\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}
