package tui

import (
	"strings"
	"testing"
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
