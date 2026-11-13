package mcpserver

import (
	"bytes"
	"strings"
	"sync"
	"time"
)

// logCapacity is how many lines the game log keeps (plan D9).
const logCapacity = 5000

// maxLogLine cuts a longer line, so one runaway print cannot fill memory.
const maxLogLine = 8 << 10

// logLine is one line of the game log.
type logLine struct {
	Seq    uint64    `json:"seq"`
	Stream string    `json:"stream"` // stdout, stderr, bridge or server
	Time   time.Time `json:"time"`
	Text   string    `json:"text"`
}

// logRing keeps the last [logCapacity] lines of the game's output, each with
// a sequence number, so a reader polls with the cursor it got last.
type logRing struct {
	mu    sync.Mutex
	lines []logLine
	next  uint64 // sequence of the next line, from 1
}

func newLogRing() *logRing { return &logRing{next: 1} }

// add appends one line.
func (r *logRing) add(stream, text string) {
	if len(text) > maxLogLine {
		text = text[:maxLogLine] + "... (cut)"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, logLine{Seq: r.next, Stream: stream, Time: time.Now(), Text: text})
	r.next++
	if over := len(r.lines) - logCapacity; over > 0 {
		r.lines = append(r.lines[:0], r.lines[over:]...)
	}
}

// read returns up to limit lines with a sequence above since, and the cursor
// to pass next time. dropped is true when lines after since were pushed out
// of the ring before they were read.
func (r *logRing) read(since uint64, limit int) (lines []logLine, next uint64, dropped bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	next = since
	if len(r.lines) > 0 && r.lines[0].Seq > since+1 {
		dropped = true
	}
	for _, l := range r.lines {
		if l.Seq <= since {
			continue
		}
		if limit > 0 && len(lines) >= limit {
			break
		}
		lines = append(lines, l)
		next = l.Seq
	}
	return lines, next, dropped
}

// tail returns the text of the last n lines, for an error message.
func (r *logRing) tail(n int) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	from := max(0, len(r.lines)-n)
	var sb strings.Builder
	for _, l := range r.lines[from:] {
		sb.WriteString(l.Text)
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}

// lineWriter is an io.Writer that adds each complete line it is given to a
// ring as stream. It is the Stdout and Stderr of the game process.
type lineWriter struct {
	mu     sync.Mutex
	ring   *logRing
	stream string
	part   []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.part = append(w.part, p...)
	for {
		i := bytes.IndexByte(w.part, '\n')
		if i < 0 {
			break
		}
		w.ring.add(w.stream, strings.TrimRight(string(w.part[:i]), "\r"))
		w.part = w.part[i+1:]
	}
	if len(w.part) > 2*maxLogLine { // a line with no end: flush it cut
		w.ring.add(w.stream, string(w.part))
		w.part = nil
	}
	return len(p), nil
}

// flush logs a last line that has no newline.
func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.part) > 0 {
		w.ring.add(w.stream, string(w.part))
		w.part = nil
	}
}
