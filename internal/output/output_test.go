package output_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	chronos "chronos.sh/go"

	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/output"
)

var now = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

type row struct {
	ID     string
	Name   string
	Status string
	Next   *time.Time
}

func sample() *chronos.Page[row] {
	next := now.Add(90 * time.Minute)
	return &chronos.Page[row]{
		Data: []row{
			{"s_1", "Nightly report for the finance team", "active", &next},
			{"s_2", "Hourly sync", "paused", nil},
		},
		Paging: chronos.Paging{Page: 1, Pages: 3, Size: 2, Total: 6},
	}
}

var columns = []output.Column[row]{
	{Header: "ID", Value: func(r row) any { return r.ID }},
	{Header: "Name", Value: func(r row) any { return r.Name }, Flex: true},
	{Header: "Status", Value: func(r row) any { return r.Status }, Status: true},
	{Header: "Next run", Value: func(r row) any { return r.Next }},
}

func printer(tty, color bool, width int) (*output.Printer, *bytes.Buffer) {
	var out bytes.Buffer
	return &output.Printer{Out: &out, Err: &out, TTY: tty, Color: color, Width: width, Now: func() time.Time { return now }}, &out
}

func TestListOnATerminal(t *testing.T) {
	p, out := printer(true, false, 50)
	if err := p.Print(output.List(sample(), "schedules", columns)); err != nil {
		t.Fatal(err)
	}
	want := `Showing 2 of 6 schedules

ID   NAME                         STATUS  NEXT RUN
s_1  Nightly report for the f...  active  in 1h
s_2  Hourly sync                  paused
`
	if out.String() != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}

func TestListColorsOnlyStatus(t *testing.T) {
	p, out := printer(true, true, 0)
	p.Print(output.List(sample(), "schedules", columns))
	if !strings.Contains(out.String(), "\x1b[32mactive\x1b[0m") || strings.Contains(out.String(), "\x1b[32mNightly") {
		t.Errorf("unexpected coloring:\n%q", out)
	}
}

func TestListPiped(t *testing.T) {
	p, out := printer(false, false, 0)
	p.Print(output.List(sample(), "schedules", columns))
	want := "s_1\tNightly report for the finance team\tactive\t2026-10-28T13:30:00Z\ns_2\tHourly sync\tpaused\t\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestListJSON(t *testing.T) {
	p, out := printer(false, false, 0)
	p.JSON = true
	p.Print(output.List(sample(), "schedules", columns))
	if !strings.HasPrefix(out.String(), `{"data":[`) || !strings.Contains(out.String(), `"paging":{"page":1,"pages":3,"size":2,"total":6}`) {
		t.Errorf("got %s", out)
	}
}

func TestResource(t *testing.T) {
	fields := func(r row) []output.Field {
		return []output.Field{{Label: "Name", Value: r.Name}, {Label: "Next run", Value: r.Next}, {Label: "Payload", Value: "line one\nline two"}}
	}
	r := sample().Data[1]

	p, out := printer(true, false, 0)
	p.Print(output.Resource(r, fields))
	want := "Name      Hourly sync\nNext run  -\nPayload   line one\n          line two\n"
	if out.String() != want {
		t.Errorf("terminal: got %q, want %q", out, want)
	}

	p, out = printer(false, false, 0)
	p.Print(output.Resource(r, fields))
	want = "name:\tHourly sync\nnext run:\t\npayload:\tline one line two\n"
	if out.String() != want {
		t.Errorf("piped: got %q, want %q", out, want)
	}
}

func TestNotesStayOffJSONStderr(t *testing.T) {
	p, out := printer(false, false, 0)
	p.JSON = true
	p.Note("Schedule %s is already paused.", "s_1")
	if out.Len() != 0 {
		t.Errorf("note printed with --json: %q", out)
	}
}

func TestRelativeTime(t *testing.T) {
	tests := []struct {
		at   time.Time
		want string
	}{
		{now.Add(-20 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(3 * time.Hour), "in 3h"},
		{now.Add(-50 * time.Hour), "2d ago"},
		{time.Date(2026, 1, 2, 12, 0, 0, 0, time.Local), "2026-01-02"},
	}
	for _, tt := range tests {
		if got := output.RelativeTime(tt.at, now); got != tt.want {
			t.Errorf("RelativeTime(%v) = %q, want %q", tt.at, got, tt.want)
		}
	}
}

func TestMaskedHeaders(t *testing.T) {
	got := output.MaskedHeaders(map[string]string{"X-Token": "secret", "Authorization": "Bearer secret"})
	if got != "Authorization: ••••, X-Token: ••••" {
		t.Errorf("got %q", got)
	}
}

func TestAPITextCannotDriveTheTerminal(t *testing.T) {
	page := &chronos.Page[row]{Data: []row{{"s_1", "Night\x1b[8mly\rX\u009b2J", "active", nil}}, Paging: chronos.Paging{Page: 1, Pages: 1, Total: 1}}
	for _, tty := range []bool{true, false} {
		p, out := printer(tty, false, 0)
		p.Print(output.List(page, "schedules", columns))
		if strings.ContainsAny(out.String(), "\x1b\r\u009b") {
			t.Errorf("tty=%v: control characters reached the output: %q", tty, out)
		}
		if !strings.Contains(out.String(), `Night\x1b[8mly\x0dX\x9b2J`) {
			t.Errorf("tty=%v: got %q", tty, out)
		}
	}
}

func TestPointerValues(t *testing.T) {
	type cell struct{ Interval *chronos.Interval }
	cols := []output.Column[cell]{{Header: "Every", Value: func(c cell) any { return c.Interval }}}
	page := &chronos.Page[cell]{Data: []cell{{nil}, {&chronos.Interval{Value: 5, Unit: chronos.IntervalUnitMinute}}}}
	p, out := printer(false, false, 0)
	p.Print(output.List(page, "schedules", cols))
	if out.String() != "\n{5 minute}\n" {
		t.Errorf("got %q", out)
	}
}

func TestNarrowFlexColumnIsNotWidened(t *testing.T) {
	short := &chronos.Page[row]{Data: []row{{"s_1_with_a_long_identifier", "Sync", "active", nil}}, Paging: chronos.Paging{Total: 1}}
	p, out := printer(true, false, 30)
	p.Print(output.List(short, "schedules", columns))
	if !strings.Contains(out.String(), "  Sync  ") {
		t.Errorf("flex column changed width:\n%s", out)
	}
}

func TestFlexColumnsShrinkRightmostFirst(t *testing.T) {
	cols := []output.Column[row]{
		{Header: "ID", Value: func(r row) any { return r.ID }},
		{Header: "Name", Value: func(r row) any { return r.Name }, Flex: true},
		{Header: "Status", Value: func(r row) any { return r.Status }, Flex: true},
	}
	long := &chronos.Page[row]{
		Data:   []row{{"s_1", "Nightly report for the finance team", strings.Repeat("x", 40), nil}},
		Paging: chronos.Paging{Total: 1},
	}
	p, out := printer(true, false, 40)
	p.Print(output.List(long, "schedules", cols))
	want := "s_1  Nightly report for...  xxxxxxxxx..."
	if !strings.Contains(out.String(), want) {
		t.Errorf("got\n%s\nwant a row %q", out, want)
	}
}

func TestDeleted(t *testing.T) {
	p, out := printer(false, false, 0)
	p.JSON = true
	p.Print(output.Deleted("schedule", "s_1"))
	if out.String() != `{"id":"s_1","deleted":true}`+"\n" {
		t.Errorf("got %s", out)
	}
}

func TestJSONTextKeepsLargeIntegersAndHTML(t *testing.T) {
	if got := output.JSONText(map[string]any{"n": json.Number("9007199254740993"), "h": "<b>"}); got != `{"h":"<b>","n":9007199254740993}` {
		t.Errorf("got %s", got)
	}
}

func TestCleanEscapesBidiAndKeepsErrorsOnOneLine(t *testing.T) {
	for in, want := range map[string]string{"prod\u202eod": `prod\u202eod`, "a\U0001D1733": `a\U0001d1733`} {
		if got := output.Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
	var errOut bytes.Buffer
	p := &output.Printer{Out: &errOut, Err: &errOut}
	p.Error(exit.Outcome{Message: "bad\nRequest ID: req_fake"})
	if strings.Count(errOut.String(), "\n") != 1 {
		t.Errorf("an API message added lines: %q", errOut.String())
	}
}

func TestJSONErrorEscapesWhatCleanEscapesOnlyOnATerminal(t *testing.T) {
	for _, tc := range []struct {
		errTTY bool
		want   string
	}{
		{errTTY: true, want: `a\u009bb`},
		{errTTY: false, want: "a\u009bb"},
	} {
		var errOut bytes.Buffer
		p := &output.Printer{Err: &errOut, JSON: true, ErrTTY: tc.errTTY}
		p.Error(exit.Outcome{Message: "a\u009bb"})
		if !strings.Contains(errOut.String(), tc.want) {
			t.Errorf("errTTY %v: got %q, want it to contain %q", tc.errTTY, errOut.String(), tc.want)
		}
	}
}

func TestJSONOnATerminalEscapesWhatCleanEscapes(t *testing.T) {
	value := map[string]string{"error": "a\u0085b\u007fc\u202ed\U000E0001e"}

	p, out := printer(true, false, 0)
	if err := p.PrintJSON(value); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"error\": \"a\\u0085b\\u007fc\\u202ed\\udb40\\udc01e\"\n}\n"
	if out.String() != want {
		t.Errorf("terminal: got %q, want %q", out, want)
	}
	var decoded map[string]string
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil || decoded["error"] != value["error"] {
		t.Errorf("terminal JSON decodes to %q, %v; want the original text", decoded["error"], err)
	}

	p, out = printer(false, false, 0)
	if err := p.PrintJSON(value); err != nil {
		t.Fatal(err)
	}
	if want := `{"error":"` + value["error"] + `"}` + "\n"; out.String() != want {
		t.Errorf("piped: got %q, want %q", out, want)
	}
}

func TestTableColumnsAlignByDisplayWidth(t *testing.T) {
	cols := []output.Column[row]{
		{Header: "ID", Value: func(r row) any { return r.ID }},
		{Header: "Name", Value: func(r row) any { return r.Name }},
		{Header: "Status", Value: func(r row) any { return r.Status }},
	}
	page := &chronos.Page[row]{
		Data:   []row{{"s_1", "日本語の名前", "active", nil}, {"s_2", "Cafe\u0301", "paused", nil}},
		Paging: chronos.Paging{Total: 2},
	}
	p, out := printer(true, false, 0)
	p.Print(output.List(page, "schedules", cols))
	want := "Showing 2 of 2 schedules\n\n" +
		"ID   NAME          STATUS\n" +
		"s_1  日本語の名前  active\n" +
		"s_2  Cafe\u0301          paused\n"
	if out.String() != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}

func TestTruncateFitsDisplayWidth(t *testing.T) {
	tests := []struct {
		in    string
		width int
		want  string
	}{
		{"日本語の名前", 7, "日本..."},
		{"abcdef", 3, "..."},
		{"abcdef", 2, "ab"},
		{"日本", 1, ""},
		{"abc", 3, "abc"},
	}
	for _, tt := range tests {
		if got := output.Truncate(tt.in, tt.width); got != tt.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.in, tt.width, got, tt.want)
		}
	}
}

func TestRedactedURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://user:secret@example.com/hook?token=abc": "https://user:xxxxx@example.com/hook?token=abc",
		"https://user@example.com/hook":                  "https://user@example.com/hook",
		"https://example.com/hook?token=abc":             "https://example.com/hook?token=abc",
		"https://user:secret@example.com:port/hook":      output.Masked,
		"user:secret@example.com/hook":                   output.Masked,
		"https:user:secret@example.com/hook":             output.Masked,
		"https:/user:secret@example.com/hook":            output.Masked,
		"/hook":                                          "/hook",
	} {
		if got := output.RedactedURL(in); got != want {
			t.Errorf("RedactedURL(%q) = %q, want %q", in, got, want)
		}
	}
}
