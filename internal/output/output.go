// Package output prints command results: JSON with --json, otherwise tables
// and key-value lines for a terminal, or tab-separated values when piped.
// Only the JSON is a stable interface.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"

	"chronos.sh/cli/internal/exit"
)

// Printer writes results to Out and notes and errors to Err.
type Printer struct {
	Out io.Writer
	Err io.Writer
	// JSON prints JSON on Out and keeps Err for JSON errors only.
	JSON bool
	// TTY is true when Out is a terminal.
	TTY    bool
	ErrTTY bool
	// Color colors status values on Out.
	Color bool
	// Width is the terminal width, or 0 when unknown.
	Width int
	Now   func() time.Time
}

type View interface {
	// JSONValue is what --json prints.
	JSONValue() any
	// Render prints the result for a person, or as TSV when piped.
	Render(p *Printer) error
}

func (p *Printer) Print(v View) error {
	if p.JSON {
		return p.PrintJSON(v.JSONValue())
	}
	return v.Render(p)
}

// PrintJSON writes v to Out, indented on a terminal and compact otherwise.
// On a terminal it also escapes the characters Clean escapes, which
// encoding/json leaves raw.
func (p *Printer) PrintJSON(v any) error {
	if !p.TTY {
		return encodeCompact(p.Out, v)
	}
	var b strings.Builder
	if err := encodeIndented(&b, v); err != nil {
		return err
	}
	_, err := io.WriteString(p.Out, escapeUnsafe(b.String()))
	return err
}

// escapeUnsafe relies on unsafe characters occurring only inside JSON strings,
// where \uXXXX decodes to the same text.
func escapeUnsafe(encoded string) string {
	if !strings.ContainsFunc(encoded, isUnsafe) {
		return encoded
	}
	var b strings.Builder
	for _, r := range encoded {
		if !isUnsafe(r) {
			b.WriteRune(r)
			continue
		}
		if r1, r2 := utf16.EncodeRune(r); r1 != unicode.ReplacementChar {
			fmt.Fprintf(&b, `\u%04x\u%04x`, r1, r2)
			continue
		}
		fmt.Fprintf(&b, `\u%04x`, r)
	}
	return b.String()
}

// Note prints a message for a person on Err. Notes are dropped with --json,
// where Err carries only the JSON error.
func (p *Printer) Note(format string, args ...any) {
	if p.JSON {
		return
	}
	fmt.Fprintln(p.Err, Clean(fmt.Sprintf(format, args...)))
}

func (p *Printer) Error(o exit.Outcome) {
	if p.JSON {
		var b strings.Builder
		encodeCompact(&b, map[string]exit.Outcome{"error": o})
		text := b.String()
		if p.ErrTTY {
			text = escapeUnsafe(text)
		}
		io.WriteString(p.Err, text)
		return
	}
	fmt.Fprintf(p.Err, "Error: %s\n", CleanLine(exit.Sentence(o.Message)))
	for _, fe := range o.FieldErrors {
		field := "(request)"
		if fe.Field != nil {
			field = CleanLine(*fe.Field)
		}
		fmt.Fprintf(p.Err, "  %s: %s\n", field, CleanLine(fe.Message))
	}
	if o.RequestID != nil {
		fmt.Fprintf(p.Err, "Request ID: %s\n", CleanLine(*o.RequestID))
	}
}

func encodeCompact(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func encodeIndented(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Clean makes text from the API safe to print on a terminal: control and
// bidi characters other than newline and tab show as \xNN, \uNNNN or
// \UNNNNNNNN instead of being interpreted, so a crafted name cannot hide or
// reorder output.
func Clean(s string) string {
	if !strings.ContainsFunc(s, isUnsafe) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case !isUnsafe(r):
			b.WriteRune(r)
		case r <= 0xff:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r <= 0xffff:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			fmt.Fprintf(&b, `\U%08x`, r)
		}
	}
	return b.String()
}

func isUnsafe(r rune) bool {
	if r == '\n' || r == '\t' {
		return false
	}
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
}

// CleanLine is Clean for text that must stay on one line, such as a prompt
// or an error message, so it cannot add lines that look like ours.
func CleanLine(s string) string {
	return lineBreaks.Replace(Clean(s))
}

func ViewOf(v any, render func(p *Printer) error) View {
	return customView{value: v, render: render}
}

type customView struct {
	value  any
	render func(p *Printer) error
}

func (v customView) JSONValue() any          { return v.value }
func (v customView) Render(p *Printer) error { return v.render(p) }

// Deleted is the result of deleting a resource, which the API answers with no
// body.
func Deleted(noun, id string) View {
	value := struct {
		ID      string `json:"id"`
		Deleted bool   `json:"deleted"`
	}{id, true}
	return ViewOf(value, func(p *Printer) error {
		_, err := fmt.Fprintf(p.Out, "Deleted %s %s\n", noun, Clean(id))
		return err
	})
}

// format turns a cell or field value into safe text. Times are relative on a
// terminal and RFC 3339 when piped; nil pointers are empty.
func (p *Printer) format(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return Clean(v)
	case int:
		return strconv.Itoa(v)
	case time.Time:
		return p.formatTime(v)
	}
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return ""
		}
		return p.format(rv.Elem().Interface())
	}
	return Clean(fmt.Sprint(v))
}

func (p *Printer) formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if !p.TTY {
		return t.UTC().Format(time.RFC3339)
	}
	return RelativeTime(t, p.Now())
}

// RelativeTime describes t from now, such as "5m ago" or "in 2h". Past 30
// days it gives the date.
func RelativeTime(t, now time.Time) string {
	d := now.Sub(t)
	future := d < 0
	if future {
		d = -d
	}
	var amount string
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		amount = strconv.Itoa(int(d/time.Minute)) + "m"
	case d < 24*time.Hour:
		amount = strconv.Itoa(int(d/time.Hour)) + "h"
	case d < 30*24*time.Hour:
		amount = strconv.Itoa(int(d/(24*time.Hour))) + "d"
	default:
		return t.Local().Format("2006-01-02")
	}
	if future {
		return "in " + amount
	}
	return amount + " ago"
}

func JSONText(v any) string {
	var b strings.Builder
	if err := encodeCompact(&b, v); err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

const Masked = "••••"

// RedactedURL hides the password in a URL such as https://user:pw@host/. A URL
// with no readable host that contains an @ may still hold one, so it returns
// Masked.
func RedactedURL(s string) string {
	u, err := url.Parse(s)
	unreadable := err != nil || u.Host == ""
	if unreadable && strings.Contains(s, "@") {
		return Masked
	}
	if unreadable {
		return s
	}
	if _, ok := u.User.Password(); !ok {
		return s
	}
	return u.Redacted()
}

// MaskedHeaders lists header names with their values hidden, since push
// headers often carry credentials.
func MaskedHeaders(headers map[string]string) string {
	parts := []string{}
	for _, name := range slices.Sorted(maps.Keys(headers)) {
		parts = append(parts, name+": "+Masked)
	}
	return strings.Join(parts, ", ")
}
