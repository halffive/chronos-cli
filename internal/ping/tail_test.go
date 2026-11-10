package ping_test

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"chronos.sh/cli/internal/ping"
)

func TestTailKeepsShortOutputWhole(t *testing.T) {
	tail := ping.NewTail(ping.BodyMaxBytes)
	tail.Write([]byte("dumping database app\n"))
	tail.Write([]byte("done\n"))
	if got := string(tail.Bytes()); got != "dumping database app\ndone\n" {
		t.Errorf("Bytes() = %q", got)
	}
}

func TestTailMarksDroppedOutputAndStartsAtALine(t *testing.T) {
	tail := ping.NewTail(ping.BodyMaxBytes)
	for i := range 1000 {
		tail.Write([]byte(strings.Repeat(string(rune('a'+i%26)), 47) + "\n"))
	}
	body := tail.Bytes()
	if len(body) > ping.BodyMaxBytes {
		t.Fatalf("body is %d bytes, more than %d", len(body), ping.BodyMaxBytes)
	}
	marker, rest, _ := bytes.Cut(body, []byte("\n"))
	if !bytes.HasPrefix(marker, []byte("[chronos: dropped the first ")) {
		t.Errorf("marker = %q", marker)
	}
	if !bytes.HasSuffix(rest, []byte(strings.Repeat("l", 47)+"\n")) {
		t.Errorf("body does not end with the last line: %q", rest[len(rest)-60:])
	}
	for line := range strings.Lines(string(rest)) {
		if len(line) != 48 {
			t.Fatalf("body has a partial line %q", line)
		}
	}
	if want := int64(48000 - len(rest)); !strings.Contains(string(marker), ping.FormatSize(want)) {
		t.Errorf("marker %q does not say %s were dropped", marker, ping.FormatSize(want))
	}
}

func TestTailDropsAPartialCharacter(t *testing.T) {
	tail := ping.NewTail(ping.BodyMaxBytes)
	tail.Write([]byte(strings.Repeat("é", ping.BodyMaxBytes)))
	_, rest, _ := bytes.Cut(tail.Bytes(), []byte("\n"))
	if !strings.HasPrefix(string(rest), "é") {
		t.Errorf("body starts mid-character: % x", rest[:4])
	}
}

func TestTailTakesConcurrentWrites(t *testing.T) {
	tail := ping.NewTail(ping.BodyMaxBytes)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 1000 {
				tail.Write([]byte("line\n"))
			}
		})
	}
	wg.Wait()
	if tail.Written() != 4*1000*5 {
		t.Errorf("Written() = %d", tail.Written())
	}
}

func TestFormatSize(t *testing.T) {
	tests := map[int64]string{
		0: "0 bytes", 1: "1 byte", 102: "102 bytes", 1023: "1023 bytes",
		10240: "10 KB", 37990: "37.1 KB", 5 << 20: "5 MB", 3 << 40: "3 TB",
	}
	for n, want := range tests {
		if got := ping.FormatSize(n); got != want {
			t.Errorf("FormatSize(%d) = %q, want %q", n, got, want)
		}
	}
}
