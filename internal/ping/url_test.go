package ping_test

import (
	"errors"
	"testing"

	"chronos.sh/cli/internal/ping"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		raw    string
		want   error
		masked string
	}{
		{"https://ping.chronos.sh/k3Jx9Qm2", nil, "https://ping.chronos.sh/••••"},
		{"  https://ping.chronos.sh/k3Jx9Qm2\n", nil, "https://ping.chronos.sh/••••"},
		{"http://localhost:8080/k3Jx9Qm2", nil, "http://localhost:8080/••••"},
		{"http://127.0.0.1/k3Jx9Qm2", nil, "http://127.0.0.1/••••"},
		{"http://ping.localhost:4000/k3Jx9Qm2", ping.ErrInsecure, ""},
		{"http://ping.localhost.example/k3Jx9Qm2", ping.ErrInsecure, ""},
		{"http://ping.chronos.sh/k3Jx9Qm2", ping.ErrInsecure, ""},
		{"ftp://ping.chronos.sh/k3Jx9Qm2", ping.ErrNotPingURL, ""},
		{"https://ping.chronos.sh/k3Jx9Qm2/start", ping.ErrNotPingURL, ""},
		{"https://ping.chronos.sh/k3Jx9Qm2/", ping.ErrNotPingURL, ""},
		{"https://ping.chronos.sh/", ping.ErrNotPingURL, ""},
		{"https://ping.chronos.sh", ping.ErrNotPingURL, ""},
		{"https://ping.chronos.sh/k3Jx9Qm2?x=1", ping.ErrNotPingURL, ""},
		{"https://ping.chronos.sh/k3Jx9Qm2#x", ping.ErrNotPingURL, ""},
		{"https://user:pw@ping.chronos.sh/k3Jx9Qm2", ping.ErrNotPingURL, ""},
		{"k3Jx9Qm2", ping.ErrNotPingURL, ""},
		{"https://ping.chronos.sh/****", ping.ErrMaskedToken, ""},
		{"https://ping.chronos.sh/••••", ping.ErrMaskedToken, ""},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			u, err := ping.ParseURL(tt.raw)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if err == nil && u.Masked() != tt.masked {
				t.Errorf("Masked() = %q, want %q", u.Masked(), tt.masked)
			}
		})
	}
}

func TestMaskURLHidesEverySegmentButThePingSuffixes(t *testing.T) {
	tests := map[string]string{
		"https://ping.chronos.sh/k3Jx9Qm2/start":       "https://ping.chronos.sh/••••/start",
		"https://ping.chronos.sh/k3Jx9Qm2/1":           "https://ping.chronos.sh/••••/1",
		"https://ping.chronos.sh/proxy/k3Jx9Qm2/fail":  "https://ping.chronos.sh/••••/••••/fail",
		"https://ping.chronos.sh/****":                 "https://ping.chronos.sh/****",
		"https://ping.chronos.sh/k3Jx9Qm2?token=x":     "https://ping.chronos.sh/••••",
		"https://user:secret@ping.chronos.sh/k3Jx9Qm2": "https://ping.chronos.sh/••••",
		"https://ping.chronos.sh/":                     "https://ping.chronos.sh/",
		"k3Jx9Qm2":                                     "••••",
	}
	for raw, want := range tests {
		if got := ping.MaskURL(raw); got != want {
			t.Errorf("MaskURL(%q) = %q, want %q", raw, got, want)
		}
	}
}
