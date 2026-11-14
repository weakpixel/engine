// Package framelog implements `wpix framelog`. It summarizes a frame log written by game.Engine
// (Config.FrameLog or WEAKPIXEL_FRAMELOG, see doc/guide/engine/engine.md#frame-log)
// and lists the frames that look like a stutter.
//
// Usage:
//
//	wpix framelog [--skip n] <frames.csv>
//
// --skip drops the first n rows (default 60, one second at 60 FPS), where
// window creation and asset loading make every frame late.
//
// It prints the number of frames, histograms of draw_gap_ms (rounded to whole
// milliseconds) and ticks, the median draw gap, then one line per hitch: a
// frame with ticks other than 1, a draw gap outside 0.75–1.5x the median, a
// GC during the frame, or, while the world scrolls, a scroll step that differs
// from the steps around it by more than a pixel and a quarter. Logs written
// before the scroll columns existed are read without the scroll check.
//
// Exit codes: 0 ok, 1 the file cannot be read or parsed, 2 usage error.
package framelog

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/weakpixel/engine/tools/internal/cli"
)

// Options are the arguments of `wpix framelog`.
type Options struct {
	File string // the frame log, a CSV file
	Skip int    // rows to drop at the start; the command's default is 60
}

// Run summarizes the log named by opts to stdout. A negative Skip is a usage
// error; a file that cannot be read or parsed, or has no rows after Skip, is a
// plain error.
func Run(opts Options, stdout io.Writer) error {
	if opts.Skip < 0 {
		return cli.Usagef("--skip must not be negative, got %d", opts.Skip)
	}
	f, err := os.Open(opts.File)
	if err != nil {
		return err
	}
	defer f.Close()
	rows, hasScroll, truncated, err := readLog(f)
	if err != nil {
		return fmt.Errorf("%s: %w", opts.File, err)
	}
	if opts.Skip >= len(rows) {
		return fmt.Errorf("%s: %d rows, nothing left after --skip %d", opts.File, len(rows), opts.Skip)
	}
	if truncated {
		fmt.Fprintln(stdout, "note: the last line is incomplete (the game was killed while writing?) and was ignored")
	}
	report(stdout, rows[opts.Skip:], hasScroll)
	return nil
}

// row is one Draw of the log, with the columns the report uses.
type row struct {
	frame  int
	t, gap float64 // ms
	ticks  int
	alpha  float64
	gc     int
	scroll float64 // |(scroll_x, scroll_y)| in logical pixels
}

// readLog parses the CSV. hasScroll reports whether the log has the scroll
// columns, truncated whether an incomplete last line was dropped (a game
// killed while writing). Every unparsable cell is reported, not only the
// first.
func readLog(r io.Reader) (rows []row, hasScroll, truncated bool, err error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // checked below, so a cut-off last line can be dropped
	recs, err := cr.ReadAll()
	if err != nil {
		return nil, false, false, err
	}
	if n := len(recs); n > 1 && len(recs[n-1]) != len(recs[0]) {
		recs, truncated = recs[:n-1], true
	}
	if len(recs) < 2 {
		return nil, false, truncated, errors.New("no rows")
	}
	col := map[string]int{}
	for i, name := range recs[0] {
		col[name] = i
	}
	var errs []error
	for _, name := range []string{"frame", "t_ms", "draw_gap_ms", "ticks", "alpha", "gc"} {
		if _, ok := col[name]; !ok {
			errs = append(errs, fmt.Errorf("missing column %q", name))
		}
	}
	if len(errs) > 0 {
		return nil, false, truncated, errors.Join(errs...)
	}
	_, hasX := col["scroll_x"]
	_, hasY := col["scroll_y"]
	hasScroll = hasX && hasY
	for i, rec := range recs[1:] {
		if len(rec) != len(recs[0]) {
			errs = append(errs, fmt.Errorf("line %d: %d fields, want %d", i+2, len(rec), len(recs[0])))
			continue
		}
		num := func(name string) float64 {
			v, err := strconv.ParseFloat(rec[col[name]], 64)
			if err != nil {
				errs = append(errs, fmt.Errorf("line %d: %s: %w", i+2, name, err))
			}
			return v
		}
		r := row{
			frame: int(num("frame")), t: num("t_ms"), gap: num("draw_gap_ms"),
			ticks: int(num("ticks")), alpha: num("alpha"), gc: int(num("gc")),
		}
		if hasScroll {
			r.scroll = math.Hypot(num("scroll_x"), num("scroll_y"))
		}
		rows = append(rows, r)
	}
	return rows, hasScroll, truncated, errors.Join(errs...)
}

// scrollWindow is how many rows on each side give the expected scroll step
// of a row; local, so that a camera easing in or out is not a hitch.
const scrollWindow = 5

// report writes the summary and the hitch lines.
func report(w io.Writer, rows []row, hasScroll bool) {
	gaps := make([]float64, len(rows))
	for i, r := range rows {
		gaps[i] = r.gap
	}
	medGap := median(gaps)
	span := rows[len(rows)-1].t - rows[0].t
	fmt.Fprintf(w, "frames: %d over %.1f s, median draw gap %.2f ms\n", len(rows), span/1000, medGap)
	fmt.Fprintf(w, "draw_gap_ms: %s\n", histogram(rows, func(r row) int { return int(math.Round(r.gap)) }))
	fmt.Fprintf(w, "ticks: %s\n", histogram(rows, func(r row) int { return r.ticks }))
	if !hasScroll {
		fmt.Fprintln(w, "scroll: no scroll columns in this log, scroll check skipped")
	}

	hitches := 0
	for i, r := range rows {
		reasons := hitchReasons(rows, i, medGap, hasScroll)
		if len(reasons) == 0 {
			continue
		}
		hitches++
		prev, next := "-", "-"
		if i > 0 {
			prev = fmt.Sprintf("%.1f", rows[i-1].gap)
		}
		if i+1 < len(rows) {
			next = fmt.Sprintf("%.1f", rows[i+1].gap)
		}
		scroll := ""
		if hasScroll {
			scroll = fmt.Sprintf(" scroll %.2f", r.scroll)
		}
		fmt.Fprintf(w, "hitch frame %d at %.0f ms: gap %.1f (prev %s, next %s) ticks %d alpha %.3f%s: %s\n",
			r.frame, r.t, r.gap, prev, next, r.ticks, r.alpha, scroll, strings.Join(reasons, ", "))
	}
	perMin := 0.0
	if span > 0 {
		perMin = float64(hitches) / (span / 60000)
	}
	fmt.Fprintf(w, "hitches: %d (%.1f per minute)\n", hitches, perMin)
}

// hitchReasons returns why row i is a hitch, or nothing.
func hitchReasons(rows []row, i int, medGap float64, hasScroll bool) []string {
	r := rows[i]
	var reasons []string
	if r.ticks != 1 {
		reasons = append(reasons, fmt.Sprintf("%d ticks", r.ticks))
	}
	switch {
	case r.gap > 1.5*medGap:
		reasons = append(reasons, "late")
	case r.gap < 0.75*medGap:
		reasons = append(reasons, "early")
	}
	if i > 0 && r.gc != rows[i-1].gc {
		reasons = append(reasons, "gc")
	}
	if hasScroll {
		if want := expectedScroll(rows, i); want >= 1 && math.Abs(r.scroll-want) > max(1, want/4) {
			reasons = append(reasons, fmt.Sprintf("scroll %.2f, around %.2f", r.scroll, want))
		}
	}
	return reasons
}

// expectedScroll is the median scroll step of the rows around i (i itself
// excluded): what a steady frame would have scrolled.
func expectedScroll(rows []row, i int) float64 {
	var near []float64
	for j := max(0, i-scrollWindow); j <= min(len(rows)-1, i+scrollWindow); j++ {
		if j != i {
			near = append(near, rows[j].scroll)
		}
	}
	return median(near)
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := slices.Clone(v)
	slices.Sort(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// histogram formats "key:count" pairs in key order.
func histogram(rows []row, key func(row) int) string {
	counts := map[int]int{}
	for _, r := range rows {
		counts[key(r)]++
	}
	keys := make([]int, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%d:%d", k, counts[k])
	}
	return strings.Join(parts, " ")
}
