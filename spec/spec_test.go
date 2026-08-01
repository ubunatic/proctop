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
	if len(cfg.Keys.Quit) == 0 {
		t.Fatal("keys.quit must not be empty")
	}
	for _, k := range cfg.Keys.Quit {
		r := ResolveKey(k)
		if r == "" {
			t.Errorf("key %q resolves to empty sequence", k)
		}
		if strings.HasPrefix(k, "<") && r == k {
			t.Errorf("named alias %q is not resolved", k)
		}
	}
}

func TestSpecThemeTokens(t *testing.T) {
	cfg := load(t)
	// Tokens the Go code looks up; each must exist in the spec theme.
	for _, tok := range []string{"cpu", "mem", "label", "dim", "max", "min", "title"} {
		if _, ok := cfg.Theme[tok]; !ok {
			t.Errorf("theme token %q missing", tok)
		}
	}
}

func TestSpecLabels(t *testing.T) {
	cfg := load(t)
	// Labels the Go code looks up; each must exist and be non-empty.
	for _, key := range []string{"cpu", "mem", "procs", "min", "max", "avg",
		"elapsed", "interval", "hint_quit", "waiting"} {
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
		"title":   cfg.App.Title,
		"plain":   cfg.Formats.Plain,
		"color":   cfg.Formats.Color,
		"summary": cfg.Summary.Text,
	} {
		if _, err := template.New(name).Funcs(stub).Parse(text); err != nil {
			t.Errorf("template %s does not parse: %v", name, err)
		}
	}
	if cfg.Formats.Time == "" {
		t.Error("formats.time must be set")
	}
}
