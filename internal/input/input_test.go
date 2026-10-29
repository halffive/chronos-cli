package input_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	chronos "chronos.sh/go"

	"chronos.sh/cli/internal/input"
)

func TestParseInterval(t *testing.T) {
	tests := map[string]chronos.Interval{
		"300": {Value: 300, Unit: chronos.IntervalUnitSecond},
		"30s": {Value: 30, Unit: chronos.IntervalUnitSecond},
		"5m":  {Value: 5, Unit: chronos.IntervalUnitMinute},
		"2h":  {Value: 2, Unit: chronos.IntervalUnitHour},
		"1d":  {Value: 1, Unit: chronos.IntervalUnitDay},
		"1w":  {Value: 1, Unit: chronos.IntervalUnitWeek},
	}
	for in, want := range tests {
		got, err := input.ParseInterval(in)
		if err != nil || got != want {
			t.Errorf("ParseInterval(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "1.5m", "5 minutes", "-5m", "5M", "m", "99999999999"} {
		if _, err := input.ParseInterval(in); err == nil {
			t.Errorf("ParseInterval(%q) succeeded, want an error", in)
		}
	}
}

func TestParseSeconds(t *testing.T) {
	tests := map[string]int{"30": 30, "90s": 90, "2m": 120, "1h": 3600, "1d": 86400, "1w": 604800}
	for in, want := range tests {
		got, err := input.ParseSeconds(in)
		if err != nil || got != want {
			t.Errorf("ParseSeconds(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"1.5m", "100000w"} {
		if _, err := input.ParseSeconds(in); err == nil {
			t.Errorf("ParseSeconds(%q) succeeded, want an error", in)
		}
	}
}

func TestParseTimeFilter(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	tests := map[string]time.Time{
		"2026-10-01T09:00:00Z": time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC),
		"24h":                  now.Add(-24 * time.Hour),
		"30m":                  now.Add(-30 * time.Minute),
	}
	for in, want := range tests {
		got, err := input.ParseTimeFilter(in, now)
		if err != nil || !got.Equal(want) {
			t.Errorf("ParseTimeFilter(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := input.ParseTimeFilter("yesterday", now); err == nil {
		t.Error("ParseTimeFilter(yesterday) succeeded, want an error")
	}
}

func TestParsePushHeader(t *testing.T) {
	name, value, err := input.ParsePushHeader("Authorization:  Bearer a:b ")
	if err != nil || name != "Authorization" || value != "Bearer a:b" {
		t.Errorf("got %q %q %v", name, value, err)
	}
	for _, in := range []string{"no colon Bearer secret", ": Bearer secret"} {
		_, _, err := input.ParsePushHeader(in)
		if err == nil || strings.Contains(err.Error(), "Bearer") {
			t.Errorf("ParsePushHeader(%q) = %v, want an error that hides the value", in, err)
		}
	}
}

func TestReadPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload.json")
	os.WriteFile(path, []byte(`{"from":"file"}`), 0o600)

	tests := []struct {
		arg, stdin, want string
	}{
		{`{"account_number": 9007199254740993}`, "", `{"account_number":9007199254740993}`},
		{"@" + path, "", `{"from":"file"}`},
		{"@-", `{"from":"stdin"}`, `{"from":"stdin"}`},
	}
	for _, tt := range tests {
		got, err := input.ReadPayload(tt.arg, strings.NewReader(tt.stdin))
		if err != nil {
			t.Errorf("ReadPayload(%q): %v", tt.arg, err)
			continue
		}
		if b, _ := json.Marshal(got); string(b) != tt.want {
			t.Errorf("ReadPayload(%q) = %s, want %s", tt.arg, b, tt.want)
		}
	}
	for _, arg := range []string{"null", "[1]", `"text"`, `{"a":1} {"b":2}`, "@" + filepath.Join(t.TempDir(), "missing.json")} {
		if _, err := input.ReadPayload(arg, strings.NewReader("")); err == nil {
			t.Errorf("ReadPayload(%q) succeeded, want an error", arg)
		}
	}
}

func TestDecodeFile(t *testing.T) {
	var params chronos.ScheduleCreateParams
	err := input.DecodeFile("-", strings.NewReader(`{"name":"Nightly","cron":"0 3 * * *","payload":{"n":9007199254740993}}`), &params)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(params.Payload); params.Name != "Nightly" || *params.Cron != "0 3 * * *" || string(b) != `{"n":9007199254740993}` {
		t.Errorf("decoded %+v, payload %s", params, b)
	}

	err = input.DecodeFile("-", strings.NewReader(`{"name":"Nightly","crn":"0 3 * * *"}`), &params)
	if err == nil || !strings.Contains(err.Error(), "crn") {
		t.Errorf("err = %v, want the unknown field named", err)
	}
}
