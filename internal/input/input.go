// Package input parses flag values into API request values. Every error it
// returns is a usage error.
package input

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	chronos "chronos.sh/go"

	"chronos.sh/cli/internal/exit"
)

var amountPattern = regexp.MustCompile(`^(\d+)([smhdw]?)$`)

var units = map[string]struct {
	unit    chronos.IntervalUnit
	seconds int
}{
	"":  {chronos.IntervalUnitSecond, 1},
	"s": {chronos.IntervalUnitSecond, 1},
	"m": {chronos.IntervalUnitMinute, 60},
	"h": {chronos.IntervalUnitHour, 3600},
	"d": {chronos.IntervalUnitDay, 86400},
	"w": {chronos.IntervalUnitWeek, 604800},
}

func parseAmount(s string) (value int, suffix string, err error) {
	m := amountPattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, "", exit.Usagef("%q is not a duration. Use a whole number with an optional unit s, m, h, d or w, such as 30s or 5m.", s)
	}
	value, err = strconv.Atoi(m[1])
	if err != nil || value > math.MaxInt32 {
		return 0, "", exit.Usagef("The duration %q is too large.", s)
	}
	return value, m[2], nil
}

// ParseInterval parses "5m" into {5, minute}. A bare number is seconds.
func ParseInterval(s string) (chronos.Interval, error) {
	value, suffix, err := parseAmount(s)
	if err != nil {
		return chronos.Interval{}, err
	}
	return chronos.Interval{Value: value, Unit: units[suffix].unit}, nil
}

// ParseSeconds parses "90s" or "2m" into whole seconds. A bare number is
// seconds.
func ParseSeconds(s string) (int, error) {
	value, suffix, err := parseAmount(s)
	if err != nil {
		return 0, err
	}
	seconds := units[suffix].seconds
	if value > math.MaxInt32/seconds {
		return 0, exit.Usagef("The duration %q is too large.", s)
	}
	return value * seconds, nil
}

func ParseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, exit.Usagef("%q is not a time. Use RFC 3339, such as 2026-11-01T09:00:00Z.", s)
	}
	return t, nil
}

// ParseTimeFilter parses an RFC 3339 timestamp, or a duration read as that
// long before now ("24h" is a day ago).
func ParseTimeFilter(s string, now time.Time) (time.Time, error) {
	if t, err := ParseTime(s); err == nil {
		return t, nil
	}
	seconds, err := ParseSeconds(s)
	if err != nil {
		return time.Time{}, exit.Usagef("%q is not a time. Use RFC 3339, or a duration such as 24h for that long ago.", s)
	}
	return now.Add(-time.Duration(seconds) * time.Second), nil
}

// ParsePushHeader parses "Name: value".
func ParsePushHeader(s string) (name, value string, err error) {
	name, value, found := strings.Cut(s, ":")
	name = strings.TrimSpace(name)
	if !found || name == "" {
		return "", "", exit.Usagef("A push header must look like \"Name: value\". The value is not shown, since it may be a secret.")
	}
	return name, strings.TrimSpace(value), nil
}

// ReadPayload reads a JSON object given inline, as @path, or as @- for stdin.
func ReadPayload(arg string, stdin io.Reader) (map[string]any, error) {
	src, closeSrc, err := open(arg, stdin)
	if err != nil {
		return nil, err
	}
	defer closeSrc()
	var payload map[string]any
	if err := decodeOne(src, &payload); err != nil {
		return nil, exit.Usagef("The payload is not valid JSON: %v.", err)
	}
	if payload == nil {
		return nil, exit.Usagef("The payload must be a JSON object.")
	}
	return payload, nil
}

// DecodeFile decodes a full request body from a path, or from stdin when arg
// is "-", into v. Unknown fields are an error, so a typo is not dropped.
func DecodeFile(arg string, stdin io.Reader, v any) error {
	src, closeSrc, err := open("@"+arg, stdin)
	if err != nil {
		return err
	}
	defer closeSrc()
	if err := decodeOne(src, v, (*json.Decoder).DisallowUnknownFields); err != nil {
		from := "in " + arg
		if arg == "-" {
			from = "from stdin"
		}
		return exit.Usagef("The request body %s is not valid: %v.", from, err)
	}
	return nil
}

func open(arg string, stdin io.Reader) (io.Reader, func(), error) {
	noop := func() {}
	path, isRef := strings.CutPrefix(arg, "@")
	switch {
	case !isRef:
		return strings.NewReader(arg), noop, nil
	case path == "-":
		return stdin, noop, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, noop, exit.Usagef("Cannot read %s: %v.", path, errors.Unwrap(err))
	}
	return f, func() { f.Close() }, nil
}

// decodeOne decodes exactly one JSON value, keeping large integers exact.
func decodeOne(r io.Reader, v any, opts ...func(*json.Decoder)) error {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	for _, opt := range opts {
		opt(dec)
	}
	if err := dec.Decode(v); err != nil {
		return err
	}
	if err := dec.Decode(&json.RawMessage{}); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after the JSON value")
	}
	return nil
}
