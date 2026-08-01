// Package tui renders the live full-screen dashboard: current CPU/MEM of the
// watched process tree, min/max extremes, and block-character history graphs.
// All labels, colors, keys, and graph characters come from the spec.
package tui

import (
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"

	"golang.org/x/term"

	"codeberg.org/ubunatic/proctop/internal/format"
	"codeberg.org/ubunatic/proctop/internal/sampler"
	"codeberg.org/ubunatic/proctop/spec"
)

// Meta describes the watched target for the title line.
type Meta struct {
	Target  string
	RootPID int
}

// TickFunc produces the next sample. ok is false while the sampler is still
// priming; a non-nil error ends the TUI (e.g. the process tree exited).
type TickFunc func() (sampler.Sample, bool, error)

// view is the render state of the dashboard.
type view struct {
	cfg      *spec.Config
	titleTpl *template.Template
	rec      *sampler.Recorder
	meta     Meta
	interval time.Duration
	last     sampler.Sample
	have     bool
	paused   bool
	notice   string // transient status shown in the hint line (screenshot result)
}

// Run drives the dashboard until a quit key is pressed, the tick function
// fails, or the input stream closes. onSample, if non-nil, is called for
// every recorded sample (used for file export while the TUI is running).
func Run(cfg *spec.Config, interval time.Duration, rec *sampler.Recorder, meta Meta,
	tick TickFunc, onSample func(sampler.Sample)) error {

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("tui: raw mode: %w", err)
	}
	out := os.Stdout
	fmt.Fprint(out, "\x1b[?1049h\x1b[?25l") // alt screen, hide cursor
	defer func() {
		fmt.Fprint(out, "\x1b[?1049l\x1b[?25h")
		_ = term.Restore(fd, oldState)
	}()

	titleTpl, err := template.New("title").Parse(cfg.App.Title)
	if err != nil {
		return fmt.Errorf("tui: parse title template: %w", err)
	}

	keySet := func(keys []string) map[string]bool {
		m := make(map[string]bool, len(keys))
		for _, k := range keys {
			m[spec.ResolveKey(k)] = true
		}
		return m
	}
	quitKeys := keySet(cfg.Keys.Quit)
	pauseKeys := keySet(cfg.Keys.Pause)
	shotKeys := keySet(cfg.Keys.Screenshot)

	input := make(chan string)
	go func() {
		defer close(input)
		buf := make([]byte, 16)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}
			input <- string(buf[:n])
		}
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	v := &view{cfg: cfg, titleTpl: titleTpl, rec: rec, meta: meta, interval: interval}
	repaint := func() {
		cols, rows := termDims(out)
		paint(out, v.frame(cols, rows))
	}
	repaint()
	for {
		select {
		case key, open := <-input:
			switch {
			case !open || key == "\x03" || quitKeys[key]: // \x03: last-resort Ctrl-C quit
				return nil
			case pauseKeys[key]:
				// Pause freezes rendering only; sampling and export continue,
				// so toggling back fast-forwards the display with no data gap.
				v.paused = !v.paused
				if !v.paused {
					v.notice = ""
				}
				repaint()
			case shotKeys[key] && v.paused:
				v.screenshot(time.Now())
				repaint()
			}
		case <-ticker.C:
			s, ok, err := tick()
			if err != nil {
				return err
			}
			if ok {
				v.last, v.have = s, true
				if onSample != nil {
					onSample(s)
				}
			}
			if !v.paused {
				repaint()
			}
		}
	}
}

// termDims returns the terminal size, falling back to 80x24 when the size
// is unavailable or reported as zero (fresh ptys do that without an error).
func termDims(out *os.File) (cols, rows int) {
	cols, rows = 80, 24
	if c, r, err := term.GetSize(int(out.Fd())); err == nil && c > 0 && r > 0 {
		cols, rows = c, r
	}
	return cols, rows
}

// frame builds one full frame as styled lines without any cursor-control
// sequences, so the same lines can be painted to the terminal or saved as
// a screenshot.
func (v *view) frame(cols, rows int) []string {
	cfg := v.cfg
	height := cfg.Defaults.GraphHeight
	if need := 2*height + 4; rows < need {
		height = max(1, (rows-4)/2)
	}

	l := cfg.Labels
	lbl := format.Fg(cfg.Theme["label"])
	dim := format.Fg(cfg.Theme["dim"])
	off := format.Off

	var title strings.Builder
	_ = v.titleTpl.Execute(&title, struct {
		Target  string
		RootPID int
		Procs   int
	}{v.meta.Target, v.meta.RootPID, v.last.Procs})

	var elapsed string
	if v.rec.Samples > 0 {
		elapsed = format.Elapsed(v.rec.End.Sub(v.rec.Start))
	}
	right := fmt.Sprintf("%s %s · %s %s", l["interval"], v.interval, l["elapsed"], elapsed)
	rightColor := dim
	if v.paused {
		right = l["paused"] + " · " + right
		rightColor = format.Fg(cfg.Theme["max"])
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s%s%s%s%s%s", format.Fg(cfg.Theme["title"]), title.String(), off,
		pad(cols-visibleWidth(title.String())-visibleWidth(right)), rightColor+right, off))
	lines = append(lines, "")

	cpu, mem := historyValues(v.rec.History)
	if !v.have {
		lines = append(lines, dim+l["waiting"]+off)
	} else {
		lines = append(lines, statLine(l["cpu"], format.Pct(v.last.CPUPct)+"%", format.Pct(v.rec.CPUMin.Value)+"%",
			format.Pct(v.rec.CPUMax.Value)+"%", v.rec.CPUMax.Time, cfg))
		for _, g := range graph(cpu, cols-2, height, []rune(cfg.Graph.Levels)) {
			lines = append(lines, "  "+format.Fg(cfg.Theme["cpu"])+g+off)
		}
		lines = append(lines, statLine(l["mem"], format.Bytes(float64(v.last.RSS)), format.Bytes(v.rec.MemMin.Value),
			format.Bytes(v.rec.MemMax.Value), v.rec.MemMax.Time, cfg))
		for _, g := range graph(mem, cols-2, height, []rune(cfg.Graph.Levels)) {
			lines = append(lines, "  "+format.Fg(cfg.Theme["mem"])+g+off)
		}
	}
	lines = append(lines, "")

	hints := l["hint_quit"] + "  " + l["hint_pause"]
	if v.paused {
		hints += "  " + l["hint_screenshot"]
	}
	if v.notice != "" {
		hints += "  " + v.notice
	}
	lines = append(lines, lbl+hints+off)
	return lines
}

// paint writes frame lines to the terminal. Lines end with clear-to-EOL and
// the frame ends with clear-to-end so stale content never lingers.
func paint(out *os.File, lines []string) {
	var b strings.Builder
	b.WriteString("\x1b[H")
	for i, ln := range lines {
		b.WriteString(ln)
		b.WriteString("\x1b[K")
		if i < len(lines)-1 {
			b.WriteString("\r\n")
		}
	}
	b.WriteString("\x1b[J")
	fmt.Fprint(out, b.String())
}

// screenshot saves the current frame (without the hint line) in the formats
// configured in the spec and records the outcome in the notice.
func (v *view) screenshot(now time.Time) {
	cols, rows := termDims(os.Stdout)
	lines := v.frame(cols, rows)
	lines = lines[:len(lines)-2] // drop trailing blank + hint line (UI chrome)
	paths, err := saveFrames(v.cfg, v.meta, now, lines)
	if err != nil {
		v.notice = format.Fg(v.cfg.Theme["max"]) + err.Error() + format.Off
		return
	}
	v.notice = format.Fg(v.cfg.Theme["min"]) + v.cfg.Labels["saved"] + " " +
		strings.Join(paths, " ") + format.Off
}

// saveFrames writes the frame in each spec-configured format. Filenames come
// from the spec name template; existing files are never overwritten.
func saveFrames(cfg *spec.Config, meta Meta, now time.Time, lines []string) ([]string, error) {
	tpl, err := template.New("shot").Parse(cfg.Screenshot.Name)
	if err != nil {
		return nil, fmt.Errorf("tui: screenshot name template: %w", err)
	}
	var sb strings.Builder
	err = tpl.Execute(&sb, struct{ Target, Time string }{
		sanitizeName(meta.Target), now.Format(cfg.Screenshot.Time),
	})
	if err != nil {
		return nil, fmt.Errorf("tui: screenshot name: %w", err)
	}
	base := sb.String()

	var paths []string
	for _, f := range cfg.Screenshot.Formats {
		content := strings.Join(lines, "\n") + "\n"
		if f == "txt" {
			stripped := make([]string, len(lines))
			for i, ln := range lines {
				stripped[i] = strings.TrimRight(stripANSI(ln), " ")
			}
			content = strings.Join(stripped, "\n") + "\n"
		}
		path := uniquePath(base, f)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("tui: screenshot: %w", err)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// uniquePath returns base.ext, or base-2.ext, base-3.ext, … if taken.
func uniquePath(base, ext string) string {
	path := base + "." + ext
	for n := 2; ; n++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path
		}
		path = fmt.Sprintf("%s-%d.%s", base, n, ext)
	}
}

// sanitizeName makes a watch target safe for filenames.
func sanitizeName(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ' ', ':':
			return '-'
		}
		return r
	}, s)
}

// statLine builds a "CPU 37.2% min 1.2% max 312.0% @15:04:05" header line.
func statLine(label, cur, minv, maxv string, maxTime time.Time, cfg *spec.Config) string {
	l := cfg.Labels
	clock := cfg.Formats.Time
	if i := strings.LastIndexAny(clock, "T "); i >= 0 {
		clock = clock[i+1:] // time-of-day part only; the date is on the title line
	}
	return fmt.Sprintf("%s%-4s%s %s%-10s%s %s%s %s%s  %s%s %s @%s%s",
		format.Fg(cfg.Theme["label"]), label, format.Off,
		format.Fg(cfg.Theme["title"]), cur, format.Off,
		format.Fg(cfg.Theme["min"]), l["min"], minv, format.Off,
		format.Fg(cfg.Theme["max"]), l["max"], maxv,
		maxTime.Format(clock), format.Off)
}

// historyValues splits the recorder history into cpu and mem series.
func historyValues(hist []sampler.Sample) (cpu, mem []float64) {
	cpu = make([]float64, len(hist))
	mem = make([]float64, len(hist))
	for i, s := range hist {
		cpu[i] = s.CPUPct
		mem[i] = float64(s.RSS)
	}
	return cpu, mem
}

// graph renders vals as height rows of block characters, latest value in the
// rightmost column, scaled to the window maximum.
func graph(vals []float64, width, height int, levels []rune) []string {
	if width < 1 || height < 1 || len(levels) < 9 {
		return nil
	}
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}
	scale := 0.0
	for _, v := range vals {
		if v > scale {
			scale = v
		}
	}
	if scale <= 0 {
		scale = 1
	}
	rows := make([][]rune, height)
	for i := range rows {
		rows[i] = make([]rune, width)
		for j := range rows[i] {
			rows[i][j] = ' '
		}
	}
	offset := width - len(vals)
	for i, v := range vals {
		units := int(v/scale*float64(height*8) + 0.5)
		if v > 0 && units == 0 {
			units = 1 // non-zero values always leave a visible mark
		}
		full, rem := units/8, units%8
		col := offset + i
		for row := 0; row < height; row++ {
			fromBottom := height - 1 - row
			switch {
			case fromBottom < full:
				rows[row][col] = levels[8]
			case fromBottom == full && rem > 0:
				rows[row][col] = levels[rem]
			}
		}
	}
	lines := make([]string, height)
	for i, r := range rows {
		lines[i] = string(r)
	}
	return lines
}

// stripANSI removes ANSI escape sequences (used for txt screenshots).
func stripANSI(s string) string {
	var sb strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case inEsc:
			if r == 'm' {
				inEsc = false
			}
		case r == '\x1b':
			inEsc = true
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// visibleWidth counts runes excluding ANSI escape sequences.
func visibleWidth(s string) int {
	return len([]rune(stripANSI(s)))
}

func pad(n int) string {
	if n < 1 {
		return " "
	}
	return strings.Repeat(" ", n)
}
