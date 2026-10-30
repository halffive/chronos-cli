package schedules_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/exit"
)

const scheduleID = "0192f3a0-0000-7000-8000-000000000001"

func fixture(t *testing.T, name string, fields map[string]any) string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(clitest.Fixture(name)))
	dec.UseNumber()
	var v map[string]any
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	for k, value := range fields {
		v[k] = value
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func schedule(t *testing.T, fields map[string]any) string {
	return fixture(t, "schedule", fields)
}

func job(t *testing.T, status string) string {
	return fixture(t, "job", map[string]any{"status": status})
}

func created(data string) clitest.Response {
	res := clitest.OK(data)
	res.Status = 201
	return res
}

func run(t *testing.T, api *clitest.API, args ...string) clitest.Result {
	t.Helper()
	return clitest.Run(t, clitest.Options{Args: args, Env: api.Env()})
}

func transcript(api *clitest.API, res clitest.Result) string {
	var b strings.Builder
	for _, r := range api.Requests() {
		fmt.Fprintf(&b, "> %s %s\n", r.Method, r.RequestURI)
		if r.Body != "" {
			fmt.Fprintf(&b, "> %s\n", r.Body)
		}
	}
	fmt.Fprintf(&b, "exit %d\n", res.Code)
	fmt.Fprintf(&b, "--- stdout\n%s", res.Stdout)
	if res.Stderr != "" {
		fmt.Fprintf(&b, "--- stderr\n%s", res.Stderr)
	}
	return strings.ReplaceAll(b.String(), api.URL, "http://api.test")
}

func wantExit(t *testing.T, res clitest.Result, want exit.Code) {
	t.Helper()
	if res.Code != want {
		t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", res.Code, want, res.Stdout, res.Stderr)
	}
}

func wantNoRequests(t *testing.T, api *clitest.API) {
	t.Helper()
	if n := len(api.Requests()); n != 0 {
		t.Errorf("the API received %d requests, want 0", n)
	}
}

func jsonError(t *testing.T, stderr string) exit.Outcome {
	t.Helper()
	var e struct {
		Error exit.Outcome `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &e); err != nil {
		t.Fatalf("stderr is not a JSON error: %v\n%s", err, stderr)
	}
	return e.Error
}
