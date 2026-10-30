package clitest

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"chronos.sh/cli/internal/exit"
)

func Created(data string) Response {
	res := OK(data)
	res.Status = http.StatusCreated
	return res
}

// FixtureWith returns Fixture(name) with fields set over its own. Numbers
// keep every digit, so large integers stay exact.
func FixtureWith(t *testing.T, name string, fields map[string]any) string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(Fixture(name)))
	dec.UseNumber()
	var v map[string]any
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	maps.Copy(v, fields)
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func WantExit(t *testing.T, res Result, want exit.Code) {
	t.Helper()
	if res.Code != want {
		t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", res.Code, want, res.Stdout, res.Stderr)
	}
}

func DecodeError(t *testing.T, stderr string) exit.Outcome {
	t.Helper()
	var e struct {
		Error exit.Outcome `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &e); err != nil {
		t.Fatalf("stderr is not a JSON error: %v\n%s", err, stderr)
	}
	return e.Error
}

func Query(t *testing.T, req Request) url.Values {
	t.Helper()
	u, err := url.Parse(req.RequestURI)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}
