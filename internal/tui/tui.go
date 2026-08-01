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

	quitKeys := make(map[string]bool, len(cfg.Keys.Quit))
	for _, k := range cfg.Keys.Quit {
		quitKeys[spec.ResolveKey(k)] = true
	}
	pauseKeys := make(map[string]bool, len(cfg.Keys.Pause))
	for _, k := range cfg.Keys.Pause {
		pauseKeys[spec.ResolveKey(k)] = true
	}

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

	var last sampler.Sample
	var have, paused bool
	render(out, cfg, titleTpl, rec, meta, interval, last, have, paused)
	for {
		select {
		case key, open := <-input:
			if !open || key == "\x03" || quitKeys[key] { // \x03: last-resort Ctrl-C quit
				return nil
			}
			if pauseKeys[key] {
				// Pause freezes rendering only; sampling and export continue,
				// so toggling back fast-forwards the display with no data gap.
				paused = !paused
				render(out, cfg, titleTpl, rec, meta, interval, last, have, paused)
			}
		case <-ticker.C:
			s, ok, err := tick()
			if err != nil {
				return err
			}
			if ok {
				last, have = s, true
				if onSample != nil {
					onSample(s)
				}
			}
			if !paused {
				render(out, cfg, titleTpl, rec, meta, interval, last, have, paused)
			}
		}
	}
}

// render paints one full frame. Lines end with clear-to-EOL and the frame
// ends with clear-to-end so stale content never lingers.
func render(out *os.File, cfg *spec.Config, titleTpl *template.Template,
	rec *sampler.Recorder, meta Meta, interval time.Duration, last sampler.Sample, have, paused bool) {

	cols, rows := 80, 24
	if c, r, err := term.GetSize(int(out.Fd())); err == nil && c > 0 && r > 0 {
		cols, rows = c, r
	}
	height := cfg.Defaults.GraphHeight
	if need := 2*height + 4; rows < need {
		height = max(1, (rows-4)/2)
	}

	l := cfg.Labels
	lbl := format.Fg(cfg.Theme["label"])
	dim := format.Fg(cfg.Theme["dim"])
	off := format.Off

	var title strings.Builder
	_ = titleTpl.Execute(&title, struct {
		Target  string
		RootPID int
		Procs   int
	}{meta.Target, meta.RootPID, last.Procs})

	var elapsed string
	if rec.Samples > 0 {
		elapsed = format.Elapsed(rec.End.Sub(rec.Start))
	}
	right := fmt.Sprintf("%s %s · %s %s", l["interval"], interval, l["elapsed"], elapsed)
	if paused {
		right = l["paused"] + " · " + right
	}

	var b strings.Builder
	b.WriteString("\x1b[H")
	line := func(s string) {
		b.WriteString(s)
		b.WriteString("\x1b[K\r\n")
	}

	rightColor := dim
	if paused {
		rightColor = format.Fg(cfg.Theme["max"])
	}
	line(fmt.Sprintf("%s%s%s%s%s%s", format.Fg(cfg.Theme["title"]), title.String(), off,
		pad(cols-visibleWidth(title.String())-visibleWidth(right)), rightColor+right, off))
	line("")

	cpu, mem := historyValues(rec.History)
	if !have {
		line(dim + l["waiting"] + off)
	} else {
		line(statLine(l["cpu"], format.Pct(last.CPUPct)+"%", format.Pct(rec.CPUMin.Value)+"%",
			format.Pct(rec.CPUMax.Value)+"%", rec.CPUMax.Time, cfg))
		for _, g := range graph(cpu, cols-2, height, []rune(cfg.Graph.Levels)) {
			line("  " + format.Fg(cfg.Theme["cpu"]) + g + off)
		}
		line(statLine(l["mem"], format.Bytes(float64(last.RSS)), format.Bytes(rec.MemMin.Value),
			format.Bytes(rec.MemMax.Value), rec.MemMax.Time, cfg))
		for _, g := range graph(mem, cols-2, height, []rune(cfg.Graph.Levels)) {
			line("  " + format.Fg(cfg.Theme["mem"]) + g + off)
		}
	}
	line("")
	b.WriteString(lbl + l["hint_quit"] + "  " + l["hint_pause"] + off + "\x1b[K\x1b[J")
	fmt.Fprint(out, b.String())
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

// visibleWidth counts runes excluding ANSI escape sequences.
func visibleWidth(s string) int {
	n := 0
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
			n++
		}
	}
	return n
}

func pad(n int) string {
	if n < 1 {
		return " "
	}
	return strings.Repeat(" ", n)
}
