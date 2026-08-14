package roscontroller

import (
	"strings"
	"sync"
	"testing"
)

// TestLogBuffer_Empty verifies that a freshly-created buffer returns no lines.
func TestLogBuffer_Empty(t *testing.T) {
	b := newLogBuffer(10)
	if got := b.Lines(0); len(got) != 0 {
		t.Errorf("empty buffer Lines(0) = %v, want []", got)
	}
	if got := b.Lines(5); len(got) != 0 {
		t.Errorf("empty buffer Lines(5) = %v, want []", got)
	}
}

// TestLogBuffer_PartialFill verifies that lines written before the buffer is
// full are returned in insertion order.
func TestLogBuffer_PartialFill(t *testing.T) {
	b := newLogBuffer(5)
	b.Write("a")
	b.Write("b")
	b.Write("c")

	got := b.Lines(0)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("Lines(0) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Lines(0)[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestLogBuffer_PartialTail verifies that the tail parameter limits the number
// of entries returned, starting from the oldest (index 0).
func TestLogBuffer_PartialTail(t *testing.T) {
	b := newLogBuffer(5)
	b.Write("a")
	b.Write("b")
	b.Write("c")

	got := b.Lines(2)
	want := []string{"a", "b"}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("Lines(2) = %v, want %v", got, want)
	}
}

// TestLogBuffer_PartialTailLargerThanContent verifies that a tail larger than
// the content returns all available lines.
func TestLogBuffer_PartialTailLargerThanContent(t *testing.T) {
	b := newLogBuffer(5)
	b.Write("a")
	b.Write("b")
	got := b.Lines(10)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("Lines(10) = %v, want [a b]", got)
	}
}

// TestLogBuffer_FullWraparound verifies that once the buffer is full, new
// writes overwrite the oldest entries and Lines returns them in correct
// chronological order (oldest → newest).
func TestLogBuffer_FullWraparound(t *testing.T) {
	b := newLogBuffer(3)
	b.Write("a")
	b.Write("b")
	b.Write("c")
	b.Write("d") // overwrites "a"
	b.Write("e") // overwrites "b"

	got := b.Lines(0)
	want := []string{"c", "d", "e"}
	if len(got) != 3 {
		t.Fatalf("Lines(0) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Lines(0)[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestLogBuffer_FullTail verifies that the tail parameter on a full buffer
// limits the count, returning entries starting from the oldest (pos).
func TestLogBuffer_FullTail(t *testing.T) {
	b := newLogBuffer(3)
	b.Write("a")
	b.Write("b")
	b.Write("c")
	b.Write("d") // overwrites "a" → buf=[d,b,c], pos=1, full

	got := b.Lines(2)
	// Oldest is at pos=1 → [b, c].
	want := []string{"b", "c"}
	if len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Errorf("Lines(2) = %v, want %v", got, want)
	}
}

// TestLogBuffer_ExactCapacity verifies that filling exactly to capacity (no
// wrap) returns all entries in order.
func TestLogBuffer_ExactCapacity(t *testing.T) {
	b := newLogBuffer(3)
	b.Write("a")
	b.Write("b")
	b.Write("c")
	// At this point pos wrapped to 0 and full=true, but content is [a,b,c].
	got := b.Lines(0)
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("exact capacity = %v, want [a b c]", got)
	}
}

// TestLogBuffer_Concurrent verifies that concurrent writes and reads do not
// race or panic. This is a smoke test, not a full correctness proof.
func TestLogBuffer_Concurrent(t *testing.T) {
	b := newLogBuffer(100)
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			b.Write(string(rune('A' + n%26)))
		}(i)
	}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Lines(10)
		}()
	}
	wg.Wait()
}

// TestProcessLogger_Write verifies that processLogger.Write prefixes the line
// and forwards it to the log buffer.
func TestProcessLogger_Write(t *testing.T) {
	buf := newLogBuffer(10)
	l := &processLogger{prefix: "[roslaunch pkg/f]", buf: buf}
	n, err := l.Write([]byte("hello\n"))
	if err != nil || n != len("hello\n") {
		t.Fatalf("Write returned n=%d err=%v", n, err)
	}
	lines := buf.Lines(0)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "[roslaunch pkg/f]") || !strings.Contains(lines[0], "hello") {
		t.Errorf("line = %q, expected prefix + content", lines[0])
	}
}
