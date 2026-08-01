package tui

import (
	"os"
	"strings"
	"testing"
	"text/template"
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

// testView builds a view with two recorded samples for frame tests.
func testView(t *testing.T) *view {
	t.Helper()
	cfg, err := spec.Load()
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	titleTpl, err := template.New("title").Parse(cfg.App.Title)
	if err != nil {
		t.Fatalf("title template: %v", err)
	}
	rec := sampler.New(100, 50)
	t0 := time.Unix(1000, 0)
	rec.Record(t0, []proc.Stat{{PID: 1, Ticks: 0, RSS: 1 << 20}})
	last, ok := rec.Record(t0.Add(time.Second), []proc.Stat{{PID: 1, Ticks: 50, RSS: 2 << 20}})
	if !ok {
		t.Fatal("second record must produce a sample")
	}
	return &view{
		cfg: cfg, titleTpl: titleTpl, rec: rec,
		meta:     Meta{Target: "firefox", RootPID: 42},
		interval: time.Second, last: last, have: true,
	}
}

func TestFrameContent(t *testing.T) {
	v := testView(t)
	lines := v.frame(90, 24)
	joined := stripANSI(strings.Join(lines, "\n"))
	for _, want := range []string{"firefox", "42", "CPU", "MEM", "50.0%", "2.0MiB", "[q]", "[p]"} {
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
	v.paused = true
	v.notice = "NOTE-XYZ"
	joined := stripANSI(strings.Join(v.frame(90, 24), "\n"))
	for _, want := range []string{v.cfg.Labels["paused"], "[s]", "NOTE-XYZ"} {
		if !strings.Contains(joined, want) {
			t.Errorf("paused frame missing %q", want)
		}
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
