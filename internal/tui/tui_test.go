package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	"codeberg.org/ubunatic/proctop/internal/proc"
	"codeberg.org/ubunatic/proctop/internal/sampler"
	"codeberg.org/ubunatic/proctop/spec"
)

var levels = []rune(" ▁▂▃▄▅▆▇█")

func TestGraphShapes(t *testing.T) {
	lines := graph([]float64{0, 50, 100}, 3, 2, levels)
	if len(lines) != 2 {
		t.Fatalf("rows = %d, want 2", len(lines))
	}
	top, bottom := []rune(lines[0]), []rune(lines[1])
	if bottom[2] != '█' || top[2] != '█' {
		t.Errorf("max value must fill the column, got top=%q bottom=%q", top[2], bottom[2])
	}
	if bottom[0] != ' ' || top[0] != ' ' {
		t.Errorf("zero value must stay empty, got top=%q bottom=%q", top[0], bottom[0])
	}
	if bottom[1] != '█' || top[1] != ' ' {
		t.Errorf("half value must fill lower half, got top=%q bottom=%q", top[1], bottom[1])
	}
}

func TestGraphWindowAndPadding(t *testing.T) {
	// More values than width: only the latest fit, aligned right.
	lines := graph([]float64{1, 2, 3, 4, 5}, 3, 1, levels)
	if got := len([]rune(lines[0])); got != 3 {
		t.Fatalf("width = %d, want 3", got)
	}
	// Fewer values than width: padded on the left.
	lines = graph([]float64{5}, 3, 1, levels)
	r := []rune(lines[0])
	if r[0] != ' ' || r[1] != ' ' || r[2] != '█' {
		t.Errorf("single value must be right-aligned, got %q", lines[0])
	}
}

func TestGraphSmallValuesVisible(t *testing.T) {
	lines := graph([]float64{0.001, 100}, 2, 1, levels)
	r := []rune(lines[0])
	if r[0] == ' ' {
		t.Error("tiny non-zero value must leave a visible mark")
	}
}

func TestGraphDegenerate(t *testing.T) {
	if g := graph(nil, 0, 0, levels); g != nil {
		t.Errorf("degenerate graph should be nil, got %v", g)
	}
	if g := graph([]float64{0, 0}, 4, 2, levels); strings.Trim(strings.Join(g, ""), " ") != "" {
		t.Errorf("all-zero graph must be empty, got %q", g)
	}
}

func TestVisibleWidth(t *testing.T) {
	if w := visibleWidth("\x1b[38;5;214mabc\x1b[0m"); w != 3 {
		t.Errorf("visibleWidth = %d, want 3", w)
	}
}

func TestStripANSI(t *testing.T) {
	if got := stripANSI("\x1b[38;5;214mCPU\x1b[0m 50%"); got != "CPU 50%" {
		t.Errorf("stripANSI = %q, want %q", got, "CPU 50%")
	}
}

// testView builds a view with several recorded samples for frame tests.
func testView(t *testing.T) *view {
	t.Helper()
	cfg, err := spec.Load()
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	rec := sampler.New(100, 50)
	t0 := time.Unix(1000, 0)
	var last sampler.Sample
	for i := 0; i <= 4; i++ {
		s, ok := rec.Record(t0.Add(time.Duration(i)*time.Second),
			[]proc.Stat{{PID: 1, Ticks: uint64(i * 50), RSS: int64(i+1) << 20}})
		if ok {
			last = s
		}
	}
	v, err := newView(cfg, rec, Meta{Target: "firefox", RootPID: 42}, time.Second)
	if err != nil {
		t.Fatalf("newView: %v", err)
	}
	v.last, v.have = last, true
	return v
}

// press feeds resolved spec keys through handleKey.
func press(t *testing.T, v *view, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if quit, _ := v.handleKey(spec.ResolveKey(k)); quit {
			t.Fatalf("key %q unexpectedly quit", k)
		}
	}
}

func TestFrameContent(t *testing.T) {
	v := testView(t)
	lines := v.frame(90, 24)
	joined := stripANSI(strings.Join(lines, "\n"))
	for _, want := range []string{"firefox", "42", "CPU", "MEM", "50.0%", "5.0MiB", "[q]", "[p]"} {
		if !strings.Contains(joined, want) {
			t.Errorf("frame missing %q in:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "[s]") {
		t.Error("screenshot hint must be hidden while not paused")
	}
	// title(1) + blank(1) + 2 × (stat + graphs) + blank(1) + hints(1)
	want := 4 + 2*(1+v.cfg.Defaults.GraphHeight)
	if len(lines) != want {
		t.Errorf("frame has %d lines, want %d", len(lines), want)
	}
}

func TestFramePaused(t *testing.T) {
	v := testView(t)
	press(t, v, "p")
	v.notice = "NOTE-XYZ"
	joined := stripANSI(strings.Join(v.frame(90, 24), "\n"))
	for _, want := range []string{v.cfg.Labels["paused"], "[s]", "[v]", "NOTE-XYZ"} {
		if !strings.Contains(joined, want) {
			t.Errorf("paused frame missing %q", want)
		}
	}
	if len(v.frozen) != len(v.rec.History) {
		t.Errorf("pause must snapshot history: %d != %d", len(v.frozen), len(v.rec.History))
	}
	if v.cursor != len(v.frozen)-1 {
		t.Errorf("cursor = %d, want last index %d", v.cursor, len(v.frozen)-1)
	}
}

func TestAnnotateFlow(t *testing.T) {
	v := testView(t)
	press(t, v, "p")                // pause: snapshot + cursor at last sample
	press(t, v, "<left>", "<left>") // move cursor two samples back
	press(t, v, "v")                // set range anchor
	press(t, v, "<left>")           // extend range one more sample
	press(t, v, "a")                // open note input
	if !v.typing {
		t.Fatal("annotate key must enter typing mode")
	}
	press(t, v, "q")     // typed chars go to the note, not quit
	v.handleKey("spike") // pasted chunk
	v.handleKey("\x7f")  // backspace: "qspike" → "qspik"
	v.handleKey("\x0d")  // enter commits
	if v.typing {
		t.Fatal("enter must leave typing mode")
	}
	if len(v.rec.Annotations) != 1 {
		t.Fatalf("got %d annotations, want 1", len(v.rec.Annotations))
	}
	a := v.rec.Annotations[0]
	if a.Note != "qspik" {
		t.Errorf("note = %q, want %q", a.Note, "qspik")
	}
	if !a.End.After(a.Start) {
		t.Errorf("range annotation must span time: %v..%v", a.Start, a.End)
	}
	// cursor: last(3) → left,left → 1 = mark anchor → left → 0; span = samples 0..1
	if hist := v.frozen; !a.Start.Equal(hist[0].Time) || !a.End.Equal(hist[1].Time) {
		t.Errorf("span %v..%v, want sample times %v..%v", a.Start, a.End, hist[0].Time, hist[1].Time)
	}

	// The annotation is pegged to timestamps: it renders in the frame list
	// and highlights columns, also after resume.
	press(t, v, "p") // resume
	joined := strings.Join(v.frame(90, 24), "\n")
	if !strings.Contains(stripANSI(joined), "qspik") {
		t.Error("annotation note missing from frame after resume")
	}
	if !strings.Contains(joined, "\x1b[48;5;") {
		t.Error("annotated columns must carry a background highlight")
	}
}

func TestAnnotateEscCancels(t *testing.T) {
	v := testView(t)
	press(t, v, "p", "a")
	v.handleKey("x")
	v.handleKey("\x1b") // esc cancels the note, does not quit
	if v.typing {
		t.Fatal("esc must leave typing mode")
	}
	if len(v.rec.Annotations) != 0 {
		t.Fatal("cancelled note must not create an annotation")
	}
	if quit, _ := v.handleKey("\x1b"); !quit {
		t.Fatal("esc outside typing must quit")
	}
}

func TestScreenshotSave(t *testing.T) {
	v := testView(t)
	v.paused = true
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(cwd)

	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	v.screenshot(now)
	v.screenshot(now) // same second: must not overwrite

	notice := stripANSI(v.notice)
	if !strings.Contains(notice, v.cfg.Labels["saved"]) {
		t.Fatalf("notice %q does not report success", notice)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	want := 2 * len(v.cfg.Screenshot.Formats)
	if len(entries) != want {
		t.Fatalf("got %d files, want %d (no overwrites)", len(entries), want)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "proctop-firefox-20260801-120000") {
			t.Errorf("unexpected filename %q", name)
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		content := string(data)
		switch {
		case strings.HasSuffix(name, ".txt"):
			if strings.Contains(content, "\x1b") {
				t.Errorf("%s contains ANSI escapes", name)
			}
			if !strings.Contains(content, "CPU") {
				t.Errorf("%s missing stats content", name)
			}
		case strings.HasSuffix(name, ".ansi"):
			if !strings.Contains(content, "\x1b[38;5;") {
				t.Errorf("%s lost its colors", name)
			}
		default:
			t.Errorf("unexpected extension on %q", name)
		}
		if strings.Contains(stripANSI(content), "[q]") {
			t.Errorf("%s contains the hint line (UI chrome)", name)
		}
	}
}
