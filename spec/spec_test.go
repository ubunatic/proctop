package spec

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
	"text/template"
	"time"
)

func load(t *testing.T) *Config {
	t.Helper()
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func TestSpecLoad(t *testing.T) {
	cfg := load(t)
	if cfg.App.Name == "" || cfg.App.Title == "" {
		t.Errorf("app name/title must be set")
	}
	if cfg.Defaults.History < 10 || cfg.Defaults.GraphHeight < 1 {
		t.Errorf("bad defaults: %+v", cfg.Defaults)
	}
	if _, err := time.ParseDuration(cfg.Defaults.Interval); err != nil {
		t.Errorf("defaults.interval: %v", err)
	}
	switch cfg.Defaults.Format {
	case "plain", "color", "json":
	default:
		t.Errorf("defaults.format %q not a known format", cfg.Defaults.Format)
	}
}

func TestSpecSchemaExists(t *testing.T) {
	data, err := fs.ReadFile(FS, "schemas/config.schema.json")
	if err != nil {
		t.Fatalf("schema missing: %v", err)
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
}

func TestSpecKeysResolve(t *testing.T) {
	cfg := load(t)
	keySets := allKeySets(cfg)
	for name, keys := range keySets {
		if len(keys) == 0 {
			t.Fatalf("keys.%s must not be empty", name)
		}
		for _, k := range keys {
			r := ResolveKey(k)
			if r == "" {
				t.Errorf("keys.%s: %q resolves to empty sequence", name, k)
			}
			if strings.HasPrefix(k, "<") && r == k {
				t.Errorf("keys.%s: named alias %q is not resolved", name, k)
			}
		}
	}
}

// allKeySets lists every key group the Go code binds; keep in sync with
// the keymaps built in internal/tui.
func allKeySets(cfg *Config) map[string][]string {
	return map[string][]string{
		"quit": cfg.Keys.Quit, "pause": cfg.Keys.Pause, "screenshot": cfg.Keys.Screenshot,
		"cursor_left": cfg.Keys.CursorLeft, "cursor_right": cfg.Keys.CursorRight,
		"mark": cfg.Keys.Mark, "annotate": cfg.Keys.Annotate,
	}
}

func TestSpecKeysNoOverlap(t *testing.T) {
	cfg := load(t)
	owner := make(map[string]string)
	for name, keys := range allKeySets(cfg) {
		for _, k := range keys {
			r := ResolveKey(k)
			if prev, taken := owner[r]; taken {
				t.Errorf("key %q bound to both %s and %s", k, prev, name)
			}
			owner[r] = name
		}
	}
}

func TestSpecScreenshot(t *testing.T) {
	cfg := load(t)
	if _, err := template.New("shot").Parse(cfg.Screenshot.Name); err != nil {
		t.Errorf("screenshot.name does not parse: %v", err)
	}
	if strings.ContainsAny(cfg.Screenshot.Time, ":/ ") {
		t.Errorf("screenshot.time %q is not filename-safe", cfg.Screenshot.Time)
	}
	if len(cfg.Screenshot.Formats) == 0 {
		t.Fatal("screenshot.formats must not be empty")
	}
	for _, f := range cfg.Screenshot.Formats {
		if f != "txt" && f != "ansi" {
			t.Errorf("screenshot format %q not supported", f)
		}
	}
}

func TestSpecThemeTokens(t *testing.T) {
	cfg := load(t)
	// Tokens the Go code looks up; each must exist in the spec theme.
	for _, tok := range []string{"cpu", "mem", "label", "dim", "max", "min", "title",
		"cursor", "select", "annot"} {
		if _, ok := cfg.Theme[tok]; !ok {
			t.Errorf("theme token %q missing", tok)
		}
	}
}

func TestSpecLabels(t *testing.T) {
	cfg := load(t)
	// Labels the Go code looks up; each must exist and be non-empty.
	for _, key := range []string{"cpu", "mem", "procs", "min", "max", "avg",
		"elapsed", "interval", "hint_quit", "hint_pause", "hint_screenshot",
		"hint_annotate", "hint_note", "note_prompt", "paused", "saved", "waiting"} {
		if cfg.Labels[key] == "" {
			t.Errorf("label %q missing or empty", key)
		}
	}
}

func TestSpecGraphLevels(t *testing.T) {
	cfg := load(t)
	if n := len([]rune(cfg.Graph.Levels)); n != 9 {
		t.Errorf("graph.levels must be 9 runes (empty + 8 levels), got %d", n)
	}
}

func TestSpecTemplatesParse(t *testing.T) {
	cfg := load(t)
	stub := template.FuncMap{
		"fg":  func(string) string { return "" },
		"off": func() string { return "" },
	}
	for name, text := range map[string]string{
		"title":      cfg.App.Title,
		"plain":      cfg.Formats.Plain,
		"color":      cfg.Formats.Color,
		"cursor":     cfg.Formats.Cursor,
		"annotation": cfg.Formats.Annotation,
		"summary":    cfg.Summary.Text,
	} {
		if _, err := template.New(name).Funcs(stub).Parse(text); err != nil {
			t.Errorf("template %s does not parse: %v", name, err)
		}
	}
	if cfg.Formats.Time == "" {
		t.Error("formats.time must be set")
	}
}
