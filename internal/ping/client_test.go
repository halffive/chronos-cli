package ping_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"chronos.sh/cli/internal/ping"
)

const token = "k3Jx9Qm2TnV7bRw4YcZ8dHfL"

type received struct {
	path, body, contentType, userAgent string
}

type server struct {
	URL *ping.URL

	mu       sync.Mutex
	statuses []int
	header   http.Header
	got      []received
}

func newServer(t *testing.T, statuses ...int) *server {
	t.Helper()
	s := &server{statuses: statuses, header: http.Header{}}
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	u, err := ping.ParseURL(ts.URL + "/" + token)
	if err != nil {
		t.Fatal(err)
	}
	s.URL = u
	return s
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.got = append(s.got, received{r.URL.Path, string(body), r.Header.Get("Content-Type"), r.Header.Get("User-Agent")})
	status := s.statuses[min(len(s.got), len(s.statuses))-1]
	for k, v := range s.header {
		w.Header()[k] = v
	}
	s.mu.Unlock()
	w.WriteHeader(status)
}

func (s *server) received() []received {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]received(nil), s.got...)
}

func client(timeout time.Duration) *ping.Client {
	c := &ping.Client{HTTPClient: &http.Client{}, UserAgent: "chronos-cli/test", Timeout: timeout}
	c.SetRetryDelay(time.Millisecond)
	return c
}

func TestPingPaths(t *testing.T) {
	s := newServer(t, http.StatusOK)
	c := client(time.Second)
	ctx := t.Context()
	if err := c.Start(ctx, s.URL); err != nil {
		t.Fatal(err)
	}
	if err := c.ExitCode(ctx, s.URL, 3, []byte("disk full\n")); err != nil {
		t.Fatal(err)
	}
	if err := c.Fail(ctx, s.URL, nil); err != nil {
		t.Fatal(err)
	}
	want := []received{
		{"/" + token + "/start", "", "text/plain; charset=utf-8", "chronos-cli/test"},
		{"/" + token + "/3", "disk full\n", "text/plain; charset=utf-8", "chronos-cli/test"},
		{"/" + token + "/fail", "", "text/plain; charset=utf-8", "chronos-cli/test"},
	}
	got := s.received()
	if len(got) != len(want) {
		t.Fatalf("received %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ping %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestPingRetries(t *testing.T) {
	tests := []struct {
		name     string
		statuses []int
		pings    int
		status   int
	}{
		{"503 then 200", []int{503, 200}, 2, 0},
		{"429 then 200", []int{429, 200}, 2, 0},
		{"404 is not retried", []int{404}, 1, 404},
		{"400 is not retried", []int{400}, 1, 400},
		{"a redirect is not followed", []int{302}, 1, 302},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newServer(t, tt.statuses...)
			err := client(time.Second).ExitCode(t.Context(), s.URL, 0, []byte("out"))
			if n := len(s.received()); n != tt.pings {
				t.Errorf("sent %d pings, want %d", n, tt.pings)
			}
			if tt.status == 0 && err != nil {
				t.Errorf("err = %v, want nil", err)
			}
			if sendErr, ok := errors.AsType[*ping.SendError](err); tt.status != 0 && (!ok || sendErr.Status != tt.status) {
				t.Errorf("err = %v, want a SendError with status %d", err, tt.status)
			}
			for _, r := range s.received() {
				if r.body != "out" {
					t.Errorf("a retry sent body %q", r.body)
				}
			}
		})
	}
}

func TestPingWaitsForRetryAfterOnlyWhenItFits(t *testing.T) {
	s := newServer(t, 429, 200)
	s.header.Set("Retry-After", "1")
	started := time.Now()
	if err := client(2*time.Second).Start(t.Context(), s.URL); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < time.Second {
		t.Errorf("retried after %s, before Retry-After", elapsed)
	}

	s = newServer(t, 429, 200)
	s.header.Set("Retry-After", "60")
	err := client(time.Second).Start(t.Context(), s.URL)
	if sendErr, ok := errors.AsType[*ping.SendError](err); !ok || sendErr.Status != 429 || len(s.received()) != 1 {
		t.Errorf("err = %v after %d pings, want the 429 at once", err, len(s.received()))
	}
}

func TestPingGivesUpAtTheTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(ts.Close)
	u, _ := ping.ParseURL(ts.URL + "/" + token)
	started := time.Now()
	err := client(200*time.Millisecond).Start(t.Context(), u)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Errorf("gave up after %s, want about 200ms", elapsed)
	}
	if err == nil || err.Error() != "no response within 200ms" {
		t.Errorf("err = %v", err)
	}
}

func TestPingNetworkErrorHidesTheURL(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	u, _ := ping.ParseURL("http://" + addr + "/" + token)
	c := client(time.Second)
	c.SetRetryDelay(time.Hour)
	err = c.Start(t.Context(), u)
	if err == nil || strings.Contains(err.Error(), token) || !strings.Contains(err.Error(), "refused") {
		t.Errorf("err = %v, want connection refused without the token", err)
	}
}

func TestPingStopsWhenCancelled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(ts.Close)
	u, _ := ping.ParseURL(ts.URL + "/" + token)
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(50*time.Millisecond, cancel)
	started := time.Now()
	err := client(10*time.Second).Start(ctx, u)
	if !errors.Is(err, context.Canceled) || time.Since(started) > time.Second {
		t.Errorf("err = %v after %s, want context.Canceled at once", err, time.Since(started))
	}
}
