package output

import (
	"fmt"
	"strings"
	"unicode/utf8"

	chronos "chronos.sh/go"
)

// Column is one table column. Value returns a string, a number, a time, or a
// pointer to one of those.
type Column[T any] struct {
	Header string
	Value  func(T) any
	// Flex columns shrink to fit the terminal width.
	Flex bool
	// Status columns are colored by value.
	Status bool
}

// Field is one line of a single-resource view.
type Field struct {
	Label string
	Value any
}

// List is one page of resources. noun is plural, such as "schedules".
func List[T any](page *chronos.Page[T], noun string, columns []Column[T]) View {
	return listView[T]{page: page, noun: noun, columns: columns}
}

type listView[T any] struct {
	page    *chronos.Page[T]
	noun    string
	columns []Column[T]
}

func (v listView[T]) JSONValue() any { return v.page }

// Paging lets the caller print a next-page hint.
func (v listView[T]) Paging() chronos.Paging { return v.page.Paging }

func (v listView[T]) Render(p *Printer) error {
	rows := make([][]string, len(v.page.Data))
	for i, item := range v.page.Data {
		rows[i] = make([]string, len(v.columns))
		for j, col := range v.columns {
			rows[i][j] = singleLine(p.format(col.Value(item)))
		}
	}
	if !p.TTY {
		for _, row := range rows {
			if _, err := fmt.Fprintln(p.Out, strings.Join(row, "\t")); err != nil {
				return err
			}
		}
		return nil
	}
	if len(rows) == 0 {
		_, err := fmt.Fprintf(p.Out, "No %s found\n", v.noun)
		return err
	}
	fmt.Fprintf(p.Out, "Showing %d of %d %s\n\n", len(rows), v.page.Paging.Total, v.noun)
	return v.renderTable(p, rows)
}

func (v listView[T]) renderTable(p *Printer, rows [][]string) error {
	headers := make([]string, len(v.columns))
	widths := make([]int, len(v.columns))
	for j, col := range v.columns {
		headers[j] = strings.ToUpper(col.Header)
		widths[j] = utf8.RuneCountInString(headers[j])
		for _, row := range rows {
			widths[j] = max(widths[j], utf8.RuneCountInString(row[j]))
		}
	}
	v.fitFlex(p.Width, widths)

	lines := append([][]string{headers}, rows...)
	for i, line := range lines {
		var b strings.Builder
		for j, cell := range line {
			cell = truncate(cell, widths[j])
			pad := widths[j] - utf8.RuneCountInString(cell)
			if i > 0 && v.columns[j].Status && p.Color {
				cell = colorStatus(cell)
			}
			b.WriteString(cell)
			if j < len(line)-1 {
				b.WriteString(strings.Repeat(" ", pad+2))
			}
		}
		if _, err := fmt.Fprintln(p.Out, strings.TrimRight(b.String(), " ")); err != nil {
			return err
		}
	}
	return nil
}

const minFlexWidth = 12

// fitFlex shrinks flex columns, rightmost first, until the table fits width.
func (v listView[T]) fitFlex(width int, widths []int) {
	if width <= 0 {
		return
	}
	total := 2 * (len(widths) - 1)
	for _, w := range widths {
		total += w
	}
	if total <= width {
		return
	}
	for j := len(v.columns) - 1; j >= 0 && total > width; j-- {
		if !v.columns[j].Flex {
			continue
		}
		shrunk := max(min(minFlexWidth, widths[j]), widths[j]-(total-width))
		total -= widths[j] - shrunk
		widths[j] = shrunk
	}
}

func truncate(s string, width int) string {
	if utf8.RuneCountInString(s) <= width {
		return s
	}
	runes := []rune(s)
	return string(runes[:width-3]) + "..."
}

var lineBreaks = strings.NewReplacer("\t", " ", "\n", " ")

func singleLine(s string) string {
	return lineBreaks.Replace(s)
}

// Resource is one resource shown as labelled lines.
func Resource[T any](v T, fields func(T) []Field) View {
	return resourceView[T]{value: v, fields: fields}
}

type resourceView[T any] struct {
	value  T
	fields func(T) []Field
}

func (v resourceView[T]) JSONValue() any { return v.value }

func (v resourceView[T]) Render(p *Printer) error {
	fields := v.fields(v.value)
	if !p.TTY {
		for _, f := range fields {
			if _, err := fmt.Fprintf(p.Out, "%s:\t%s\n", strings.ToLower(f.Label), singleLine(p.format(f.Value))); err != nil {
				return err
			}
		}
		return nil
	}
	width := 0
	for _, f := range fields {
		width = max(width, utf8.RuneCountInString(f.Label))
	}
	for _, f := range fields {
		value := p.format(f.Value)
		if value == "" {
			value = "-"
		}
		value = strings.ReplaceAll(value, "\n", "\n"+strings.Repeat(" ", width+2))
		if _, err := fmt.Fprintf(p.Out, "%-*s  %s\n", width, f.Label, value); err != nil {
			return err
		}
	}
	return nil
}

const (
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	red    = "\x1b[31m"
	gray   = "\x1b[90m"
	reset  = "\x1b[0m"
)

var statusColors = map[string]string{
	"active": green, "completed": green, "online": green, "up": green,
	"paused": yellow, "pending": yellow, "admitting": yellow, "queued": yellow,
	"ready": yellow, "running": yellow, "retrying": yellow, "new": yellow, "late": yellow,
	"failed": red, "timeout": red, "down": red,
	"archived": gray, "cancelled": gray, "skipped": gray, "offline": gray,
}

func colorStatus(s string) string {
	color, ok := statusColors[s]
	if !ok {
		return s
	}
	return color + s + reset
}
