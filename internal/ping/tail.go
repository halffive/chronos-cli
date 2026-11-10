package ping

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

func marker(dropped int64) string {
	return fmt.Sprintf("[chronos: dropped the first %s of output]\n", FormatSize(dropped))
}

// Tail keeps the last bytes written to it. It is safe for concurrent writes,
// such as a command's stdout and stderr.
type Tail struct {
	mu      sync.Mutex
	max     int
	buf     []byte
	written int64
}

func NewTail(max int) *Tail {
	return &Tail{max: max}
}

func (t *Tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.written += int64(len(p))
	t.buf = append(t.buf, p...)
	// buf may hold up to twice max, so a full tail moves its bytes once per
	// max bytes written rather than on every write.
	if len(t.buf) > 2*t.max {
		t.buf = append(t.buf[:0], t.buf[len(t.buf)-t.max:]...)
	}
	return len(p), nil
}

// Written is how many bytes were written, kept or not.
func (t *Tail) Written() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.written
}

// Bytes returns everything written when it fits. Otherwise it returns a
// marker line saying how much was dropped, then the last bytes from the start
// of a line, together at most max bytes.
func (t *Tail) Bytes() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.written <= int64(t.max) {
		return bytes.Clone(t.buf)
	}
	// reserve room for the longest marker, "1023 bytes".
	start := len(t.buf) - max(t.max-len(marker(1023)), 0)
	kept := t.buf[start:]
	if t.buf[start-1] != '\n' {
		kept = fromNextLine(kept)
	}
	kept = trimPartialRune(kept)
	return append([]byte(marker(t.written-int64(len(kept)))), kept...)
}

// fromNextLine drops b's first, partial line, unless that would leave nothing.
func fromNextLine(b []byte) []byte {
	i := bytes.IndexByte(b, '\n')
	if i < 0 || i == len(b)-1 {
		return b
	}
	return b[i+1:]
}

func trimPartialRune(b []byte) []byte {
	for i := 0; i < utf8.UTFMax-1 && len(b) > 0 && !utf8.RuneStart(b[0]); i++ {
		b = b[1:]
	}
	return b
}

// FormatSize writes a byte count for people: 102 bytes, 10 KB, 37.1 KB.
func FormatSize(n int64) string {
	if n == 1 {
		return "1 byte"
	}
	if n < 1024 {
		return strconv.FormatInt(n, 10) + " bytes"
	}
	v := float64(n)
	units := []string{"KB", "MB", "GB", "TB"}
	for i, unit := range units {
		v /= 1024
		if v < 1024 || i == len(units)-1 {
			return strings.TrimSuffix(strconv.FormatFloat(v, 'f', 1, 64), ".0") + " " + unit
		}
	}
	return ""
}
