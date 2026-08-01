// Package tui renders the live full-screen dashboard: current CPU/MEM of the
// watched process tree, min/max extremes, and block-character history graphs.
// In pause mode a column cursor allows annotating sample spans and saving
// screenshots. All labels, colors, keys, and graph characters come from the
// spec.
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

// keymaps holds the resolved key sets from the spec.
type keymaps struct {
	quit, pause, shot           map[string]bool
	left, right, mark, annotate map[string]bool
}

func newKeymaps(cfg *spec.Config) keymaps {
	set := func(keys []string) map[string]bool {
		m := make(map[string]bool, len(keys))
		for _, k := range keys {
			m[spec.ResolveKey(k)] = true
		}
		return m
	}
	return keymaps{
		quit:     set(cfg.Keys.Quit),
		pause:    set(cfg.Keys.Pause),
		shot:     set(cfg.Keys.Screenshot),
		left:     set(cfg.Keys.CursorLeft),
		right:    set(cfg.Keys.CursorRight),
		mark:     set(cfg.Keys.Mark),
		annotate: set(cfg.Keys.Annotate),
	}
}

// view is the render and interaction state of the dashboard.
type view struct {
	cfg       *spec.Config
	titleTpl  *template.Template
	cursorTpl *template.Template
	annotTpl  *template.Template
	keys      keymaps
	rec       *sampler.Recorder
	meta      Meta
	interval  time.Duration

	last   sampler.Sample
	have   bool
	paused bool
	notice string // transient status shown in the hint line (screenshot result)

	frozen []sampler.Sample // pause-time history snapshot; annotations peg to its timestamps
	cursor int              // index into frozen, -1 = none
	mark   int              // selection anchor index into frozen, -1 = none
	typing bool             // note input mode
	draft  string           // note text being typed
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

	v, err := newView(cfg, rec, meta, interval)
	if err != nil {
		return err
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

	repaint := func() {
		cols, rows := termDims(out)
		paint(out, v.frame(cols, rows))
	}
	repaint()
	for {
		select {
		case key, open := <-input:
			if !open {
				return nil
			}
			quit, dirty := v.handleKey(key)
			if quit {
				return nil
			}
			if dirty {
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

// newView parses the spec templates and prepares the interaction state.
func newView(cfg *spec.Config, rec *sampler.Recorder, meta Meta, interval time.Duration) (*view, error) {
	titleTpl, err := template.New("title").Parse(cfg.App.Title)
	if err != nil {
		return nil, fmt.Errorf("tui: parse title template: %w", err)
	}
	cursorTpl, err := template.New("cursor").Parse(cfg.Formats.Cursor)
	if err != nil {
		return nil, fmt.Errorf("tui: parse cursor template: %w", err)
	}
	annotTpl, err := template.New("annotation").Parse(cfg.Formats.Annotation)
	if err != nil {
		return nil, fmt.Errorf("tui: parse annotation template: %w", err)
	}
	return &view{
		cfg: cfg, titleTpl: titleTpl, cursorTpl: cursorTpl, annotTpl: annotTpl,
		keys: newKeymaps(cfg), rec: rec, meta: meta, interval: interval,
		cursor: -1, mark: -1,
	}, nil
}

// handleKey processes one key chunk. quit ends the TUI; dirty requests a
// repaint. Enter/Esc/Backspace during note input and Ctrl-C are structural
// keys and stay hardcoded (Ctrl-C as the last-resort quit safeguard).
func (v *view) handleKey(key string) (quit, dirty bool) {
	km := v.keys
	switch {
	case key == "\x03": // structural: last-resort Ctrl-C quit
		return true, false
	case v.typing:
		switch key {
		case "\x0d": // structural: Enter commits the note
			v.commitNote()
		case "\x1b": // structural: Esc cancels the note (not quit)
			v.typing, v.draft = false, ""
		case "\x7f": // structural: Backspace edits the note
			r := []rune(v.draft)
			if len(r) > 0 {
				v.draft = string(r[:len(r)-1])
			}
		default:
			if printable(key) && len(v.draft) < 80 {
				v.draft += key
			}
		}
		return false, true
	case km.quit[key]:
		return true, false
	case km.pause[key]:
		v.paused = !v.paused
		if v.paused {
			// Snapshot the history: sampling continues in the background,
			// but cursor positions and annotations need stable timestamps.
			v.frozen = append([]sampler.Sample(nil), v.rec.History...)
			v.cursor = len(v.frozen) - 1
		} else {
			v.frozen, v.cursor, v.mark = nil, -1, -1
			v.notice = ""
		}
		return false, true
	case !v.paused:
		return false, false
	case km.shot[key]:
		v.screenshot(time.Now())
		return false, true
	case km.left[key]:
		return false, v.moveCursor(-1)
	case km.right[key]:
		return false, v.moveCursor(+1)
	case km.mark[key]:
		if v.cursor >= 0 {
			if v.mark >= 0 {
				v.mark = -1
			} else {
				v.mark = v.cursor
			}
			return false, true
		}
	case km.annotate[key]:
		if v.cursor >= 0 {
			v.typing, v.draft = true, ""
			return false, true
		}
	}
	return false, false
}

func (v *view) moveCursor(d int) bool {
	if len(v.frozen) == 0 {
		return false
	}
	c := v.cursor + d
	if c < 0 {
		c = 0
	}
	if c > len(v.frozen)-1 {
		c = len(v.frozen) - 1
	}
	if c == v.cursor {
		return false
	}
	v.cursor = c
	return true
}

// commitNote turns the current cursor/mark span and draft into an annotation.
// An empty draft cancels instead of committing.
func (v *view) commitNote() {
	defer func() { v.typing, v.draft, v.mark = false, "", -1 }()
	if v.draft == "" || v.cursor < 0 || v.cursor >= len(v.frozen) {
		return
	}
	lo, hi := v.cursor, v.cursor
	if v.mark >= 0 && v.mark < len(v.frozen) {
		if v.mark < lo {
			lo = v.mark
		}
		if v.mark > hi {
			hi = v.mark
		}
	}
	v.rec.Annotate(sampler.Annotation{
		Start: v.frozen[lo].Time,
		End:   v.frozen[hi].Time,
		Note:  v.draft,
	})
}

// printable reports whether a key chunk is note text (no control bytes).
func printable(key string) bool {
	if key == "" {
		return false
	}
	for _, b := range []byte(key) {
		if b < 0x20 || b == 0x7f {
			return false
		}
	}
	return true
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

	hist := v.rec.History
	if v.paused && v.frozen != nil {
		hist = v.frozen
	}
	width := cols - 2
	marks := v.columnMarks(hist, width)
	cpu, mem := historyValues(hist)
	if !v.have {
		lines = append(lines, dim+l["waiting"]+off)
	} else {
		levels := []rune(cfg.Graph.Levels)
		cpuRows := graph(cpu, width, height, levels)
		memRows := graph(mem, width, height, levels)
		overlays := v.placeLabels(hist, width, [2][]string{cpuRows, memRows})
		cpuFg := format.Fg(cfg.Theme["cpu"])
		memFg := format.Fg(cfg.Theme["mem"])
		lines = append(lines, statLine(l["cpu"], format.Pct(v.last.CPUPct)+"%", format.Pct(v.rec.CPUMin.Value)+"%",
			format.Pct(v.rec.CPUMax.Value)+"%", v.rec.CPUMax.Time, cfg))
		for ri, g := range cpuRows {
			lines = append(lines, "  "+cpuFg+renderRow(g, overlays[0][ri], marks, cpuFg)+off)
		}
		lines = append(lines, statLine(l["mem"], format.Bytes(float64(v.last.RSS)), format.Bytes(v.rec.MemMin.Value),
			format.Bytes(v.rec.MemMax.Value), v.rec.MemMax.Time, cfg))
		for ri, g := range memRows {
			lines = append(lines, "  "+memFg+renderRow(g, overlays[1][ri], marks, memFg)+off)
		}
	}
	lines = append(lines, v.infoLines(hist)...)
	lines = append(lines, "")
	lines = append(lines, v.hintLine())
	return lines
}

// window returns the first visible history index and the left padding
// offset for a graph of the given width.
func window(histLen, width int) (start, offset int) {
	if histLen > width {
		start = histLen - width
	}
	offset = width - (histLen - start)
	return start, offset
}

// columnMarks maps visible graph columns to highlight sequences: annotated
// spans (matched by sample timestamp), the selection range, and the cursor.
func (v *view) columnMarks(hist []sampler.Sample, width int) map[int]string {
	if width < 1 || len(hist) == 0 {
		return nil
	}
	start, offset := window(len(hist), width)
	colOf := func(i int) (int, bool) { // absolute history index → column
		if i < start {
			return 0, false
		}
		return offset + (i - start), true
	}
	marks := make(map[int]string)
	annotBg := format.Bg(v.cfg.Theme["annot"])
	for i, s := range hist[start:] {
		for _, a := range v.rec.Annotations {
			if a.Covers(s.Time) {
				marks[offset+i] = annotBg
				break
			}
		}
	}
	if v.paused && v.mark >= 0 && v.cursor >= 0 {
		lo, hi := v.mark, v.cursor
		if lo > hi {
			lo, hi = hi, lo
		}
		selBg := format.Bg(v.cfg.Theme["select"])
		for i := lo; i <= hi; i++ {
			if col, ok := colOf(i); ok {
				marks[col] = selBg
			}
		}
	}
	if v.paused && v.cursor >= 0 {
		if col, ok := colOf(v.cursor); ok {
			marks[col] = format.Bg(v.cfg.Theme["cursor"])
		}
	}
	return marks
}

// cell is one in-graph label character with its colors. An empty bg keeps
// the column-highlight background (if any).
type cell struct {
	r  rune
	fg string
	bg string
}

// labelGlyphs resolves the spec label style to concrete runes.
type labelGlyphs struct {
	open, close rune // single-row deco ('[', ']'; 0 = none)
	line        rune // connector line (0 = none)
	capL, capR  rune // connector tips (0 = use marker glyph instead)
	boxArt      bool // 3-row box art requested
}

func newLabelGlyphs(cfg *spec.Config) labelGlyphs {
	g := labelGlyphs{}
	switch cfg.Graph.LabelBox {
	case "brackets":
		g.open, g.close = '[', ']'
	case "box":
		g.open, g.close = '[', ']' // single-row fallback deco
		g.boxArt = true
	}
	switch cfg.Graph.LabelLine {
	case "border":
		g.line = '─'
	case "minus":
		g.line = '-'
	}
	switch cfg.Graph.LabelCap {
	case "simple":
		g.capL, g.capR = '<', '>'
	case "arrow":
		g.capL, g.capR = '◀', '▶'
	}
	return g
}

// connector returns the runes between label and column, cap-side last for
// left placement ("──▶") and cap-side first for right placement ("◀──").
func (g labelGlyphs) connector(left bool) []rune {
	if g.capL == 0 {
		return nil // cap none: the marker glyph points at the column instead
	}
	var line []rune
	if g.line != 0 {
		line = []rune{g.line, g.line}
	}
	if left {
		return append(line, g.capR)
	}
	return append([]rune{g.capL}, line...)
}

// deco wraps the note text in the single-row box decoration.
func (g labelGlyphs) deco(text []rune) []rune {
	if g.open == 0 {
		return text
	}
	out := make([]rune, 0, len(text)+2)
	out = append(out, g.open)
	out = append(out, text...)
	return append(out, g.close)
}

// placeLabels lays annotation notes into blank areas of the raw graph grids
// so they sit next to their annotated column (mock-up style: "note ▼").
// grids[0] is the CPU graph, grids[1] the MEM graph; the returned overlays
// use the same indexing (graph → row → column). The marker glyph lands on
// the annotated column; the text goes left of it when there is blank space,
// right of it otherwise. Labels that fit nowhere are skipped — the column
// highlight and the note list below still identify the annotation.
func (v *view) placeLabels(hist []sampler.Sample, width int, grids [2][]string) [2]map[int]map[int]cell {
	overlays := [2]map[int]map[int]cell{{}, {}}
	if len(hist) == 0 || width < 1 || len(v.rec.Annotations) == 0 {
		return overlays
	}
	start, offset := window(len(hist), width)
	glyphs := newLabelGlyphs(v.cfg)
	connFg := format.Fg(v.cfg.Theme["connector"]) // distinct from the annot highlight bg
	labelFg := format.Fg(v.cfg.Theme["label"])
	labelBg := format.Bg(v.cfg.Theme["label_bg"])
	marker := []rune(v.cfg.Graph.Marker)[0]

	var rows [2][][]rune
	for gi, g := range grids {
		rows[gi] = make([][]rune, len(g))
		for ri, line := range g {
			rows[gi][ri] = []rune(line)
		}
	}
	free := func(gi, row, from, to int) bool { // inclusive column range
		if from < 0 || to >= width || row < 0 || row >= len(rows[gi]) {
			return false
		}
		for c := from; c <= to; c++ {
			if rows[gi][row][c] != ' ' {
				return false
			}
			if _, used := overlays[gi][row][c]; used {
				return false
			}
		}
		return true
	}
	// Connector and marker cells may overlay bars (mock-up style: the arrow
	// touches the data); they only must not collide with another label.
	overlayOK := func(gi, row, from, to int) bool {
		if from < 0 || to >= width || row < 0 || row >= len(rows[gi]) {
			return false
		}
		for c := from; c <= to; c++ {
			if _, used := overlays[gi][row][c]; used {
				return false
			}
		}
		return true
	}
	put := func(gi, row, col int, c cell) {
		if overlays[gi][row] == nil {
			overlays[gi][row] = map[int]cell{}
		}
		overlays[gi][row][col] = c
	}
	putRunes := func(gi, row, col int, rs []rune, c cell) {
		for k, r := range rs {
			c.r = r
			put(gi, row, col+k, c)
		}
	}

	// putConn writes the connector (or the marker when cap is none) on one
	// row so that its tip lands on col. connCols reports the cells it needs.
	connCols := func(conn []rune, col int, left bool) (from, to int) {
		cw := max(len(conn), 1) // cap none: the marker occupies the column cell
		if left {
			return col - cw + 1, col
		}
		return col, col + cw - 1
	}
	putConn := func(gi, row, col int, conn []rune, left bool) {
		if len(conn) == 0 {
			put(gi, row, col, cell{r: marker, fg: connFg})
			return
		}
		from, _ := connCols(conn, col, left)
		putRunes(gi, row, from, conn, cell{fg: connFg})
	}

	// placeRow puts "deco conn" (pointing at the area's left edge) or
	// "conn deco" (pointing at its right edge) on one row.
	placeRow := func(gi, row, colL, colR int, deco []rune) bool {
		for _, left := range []bool{true, false} {
			col := colL
			if !left {
				col = colR
			}
			conn := glyphs.connector(left)
			cFrom, cTo := connCols(conn, col, left)
			dFrom := cFrom - len(deco)
			if !left {
				dFrom = cTo + 1
			}
			if free(gi, row, dFrom, dFrom+len(deco)-1) && overlayOK(gi, row, cFrom, cTo) {
				putRunes(gi, row, dFrom, deco, cell{fg: labelFg, bg: labelBg})
				putConn(gi, row, col, conn, left)
				return true
			}
		}
		return false
	}

	// placeBox puts a 3-row box-art label with the connector on its middle
	// row, pointing at the area edge like placeRow.
	placeBox := func(gi, row, colL, colR int, text []rune) bool {
		w := len(text) + 2
		top := []rune("┌" + strings.Repeat("─", len(text)) + "┐")
		mid := append(append([]rune{'│'}, text...), '│')
		bot := []rune("└" + strings.Repeat("─", len(text)) + "┘")
		boxCell := cell{fg: labelFg, bg: labelBg}
		for _, left := range []bool{true, false} {
			col := colL
			if !left {
				col = colR
			}
			conn := glyphs.connector(left)
			cFrom, cTo := connCols(conn, col, left)
			bStart := cFrom - w
			if !left {
				bStart = cTo + 1
			}
			fits := free(gi, row, bStart, bStart+w-1) && free(gi, row+1, bStart, bStart+w-1) &&
				free(gi, row+2, bStart, bStart+w-1) && overlayOK(gi, row+1, cFrom, cTo)
			if !fits {
				continue
			}
			putRunes(gi, row, bStart, top, boxCell)
			putRunes(gi, row+1, bStart, mid, boxCell)
			putRunes(gi, row+2, bStart, bot, boxCell)
			putConn(gi, row+1, col, conn, left)
			return true
		}
		return false
	}

	for _, a := range v.rec.Annotations {
		first, last := -1, -1 // visible covered sample range
		for i := start; i < len(hist); i++ {
			if a.Covers(hist[i].Time) {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first < 0 {
			continue // annotation scrolled out of view
		}
		// The label points at the area's edge: left edge when the label sits
		// before the area, right edge for the right-side fallback.
		colL := offset + (first - start)
		colR := offset + (last - start)
		text := []rune(a.Note)
		if maxw := v.cfg.Graph.LabelWidth; len(text) > maxw {
			text = append(text[:maxw-1], '…')
		}
		placed := false
		if glyphs.boxArt {
			for gi := range rows {
				for row := 0; row+2 < len(rows[gi]) && !placed; row++ {
					placed = placeBox(gi, row, colL, colR, text)
				}
				if placed {
					break
				}
			}
		}
		deco := glyphs.deco(text)
		for gi := range rows {
			for row := 0; row < len(rows[gi]) && !placed; row++ {
				placed = placeRow(gi, row, colL, colR, deco)
			}
			if placed {
				break
			}
		}
	}
	return overlays
}

// renderRow merges a raw graph line with in-graph label cells and column
// background highlights, switching colors only where needed.
func renderRow(raw string, overlay map[int]cell, marks map[int]string, baseFg string) string {
	if len(overlay) == 0 && len(marks) == 0 {
		return raw
	}
	var sb strings.Builder
	cur := baseFg
	for i, r := range []rune(raw) {
		fg, rr, bg := baseFg, r, marks[i]
		if c, ok := overlay[i]; ok {
			fg, rr = c.fg, c.r
			if c.bg != "" {
				bg = c.bg
			}
		}
		if fg != cur {
			sb.WriteString(fg)
			cur = fg
		}
		if bg != "" {
			sb.WriteString(bg)
			sb.WriteRune(rr)
			sb.WriteString("\x1b[49m") // reset background only
		} else {
			sb.WriteRune(rr)
		}
	}
	return sb.String()
}

// infoLines renders the cursor info line (paused) and the annotation list.
func (v *view) infoLines(hist []sampler.Sample) []string {
	cfg := v.cfg
	clock := timeOfDayLayout(cfg.Formats.Time)
	var lines []string
	if v.paused && v.cursor >= 0 && v.cursor < len(v.frozen) {
		s := v.frozen[v.cursor]
		var sb strings.Builder
		_ = v.cursorTpl.Execute(&sb, struct {
			Time, CPU, Mem string
		}{s.Time.Format(clock), format.Pct(s.CPUPct), format.Bytes(float64(s.RSS))})
		lines = append(lines, format.Fg(cfg.Theme["cursor"])+sb.String()+format.Off)
	}
	annotFg := format.Fg(cfg.Theme["annot"])
	for _, a := range v.rec.Annotations {
		span := ""
		if a.End.After(a.Start) {
			span = "–" + a.End.Format(clock)
		}
		cpu, rss, _ := v.rec.SpanMax(a.Start, a.End)
		var sb strings.Builder
		_ = v.annotTpl.Execute(&sb, struct {
			Time, Span, CPU, Mem, Note string
		}{a.Start.Format(clock), span, format.Pct(cpu), format.Bytes(rss), a.Note})
		lines = append(lines, annotFg+sb.String()+format.Off)
	}
	return lines
}

// hintLine renders the bottom line: key hints, note input, or notices.
func (v *view) hintLine() string {
	l := v.cfg.Labels
	lbl := format.Fg(v.cfg.Theme["label"])
	off := format.Off
	if v.typing {
		return lbl + l["note_prompt"] + " " + v.draft + "▏ " +
			format.Fg(v.cfg.Theme["dim"]) + l["hint_note"] + off
	}
	hints := l["hint_quit"] + "  " + l["hint_pause"]
	if v.paused {
		hints += "  " + l["hint_screenshot"] + "  " + l["hint_annotate"]
	}
	if v.notice != "" {
		hints += "  " + v.notice
	}
	return lbl + hints + off
}

// timeOfDayLayout strips the date part off the spec time layout.
func timeOfDayLayout(layout string) string {
	if i := strings.LastIndexAny(layout, "T "); i >= 0 {
		return layout[i+1:]
	}
	return layout
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
	clock := timeOfDayLayout(cfg.Formats.Time)
	return fmt.Sprintf("%s%-4s%s %s%-10s%s %s%s %s%s  %s%s %s @%s%s",
		format.Fg(cfg.Theme["label"]), label, format.Off,
		format.Fg(cfg.Theme["title"]), cur, format.Off,
		format.Fg(cfg.Theme["min"]), l["min"], minv, format.Off,
		format.Fg(cfg.Theme["max"]), l["max"], maxv,
		maxTime.Format(clock), format.Off)
}

// historyValues splits a sample history into cpu and mem series.
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
