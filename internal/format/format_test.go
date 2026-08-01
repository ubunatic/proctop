package format

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"codeberg.org/ubunatic/proctop/internal/proc"
	"codeberg.org/ubunatic/proctop/internal/sampler"
	"codeberg.org/ubunatic/proctop/spec"
)

func testSample() sampler.Sample {
	return sampler.Sample{
		Time:   time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
		CPUPct: 37.25,
		RSS:    2 * 1024 * 1024 * 1024,
		Procs:  42,
	}
}

func TestLineFormats(t *testing.T) {
	cfg, err := spec.Load()
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	tests := []struct {
		kind string
		want []string
	}{
		{Plain, []string{"37.2", "2.0GiB", "42", "2026-08-01T12:00:00"}},
		{Color, []string{"37.2", "2.0GiB", "\x1b[38;5;"}},
		{JSON, []string{`"cpu_pct":37.25`, `"procs":42`}},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			fn, err := New(tt.kind, cfg)
			if err != nil {
				t.Fatalf("New(%s): %v", tt.kind, err)
			}
			line, err := fn(testSample())
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(line, want) {
					t.Errorf("%s line %q missing %q", tt.kind, line, want)
				}
			}
		})
	}
	if _, err := New("nope", cfg); err == nil {
		t.Error("unknown format must fail")
	}
}

func TestJSONLineParses(t *testing.T) {
	cfg, _ := spec.Load()
	fn, _ := New(JSON, cfg)
	line, err := fn(testSample())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(line), &v); err != nil {
		t.Fatalf("json line does not parse: %v", err)
	}
}

func TestSummary(t *testing.T) {
	cfg, err := spec.Load()
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	r := sampler.New(100, 10)
	t0 := time.Unix(1000, 0)
	r.Record(t0, []proc.Stat{{PID: 1, Ticks: 0, RSS: 1024}})
	r.Record(t0.Add(time.Second), []proc.Stat{{PID: 1, Ticks: 50, RSS: 2048}})

	text, err := SummaryText(cfg, "firefox", 100, r)
	if err != nil {
		t.Fatalf("SummaryText: %v", err)
	}
	for _, want := range []string{"firefox", "100", "50.0", "2.0KiB"} {
		if !strings.Contains(text, want) {
			t.Errorf("summary %q missing %q", text, want)
		}
	}

	r.Annotate(sampler.Annotation{Start: t0, End: t0.Add(time.Second), Note: "spike"})
	js, err := SummaryJSON("firefox", 100, r)
	if err != nil {
		t.Fatalf("SummaryJSON: %v", err)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(js), &v); err != nil {
		t.Fatalf("summary json does not parse: %v", err)
	}
	if v["cpu_max_pct"] != 50.0 {
		t.Errorf("cpu_max_pct = %v, want 50", v["cpu_max_pct"])
	}
	annots, ok := v["annotations"].([]any)
	if !ok || len(annots) != 1 {
		t.Fatalf("annotations = %v, want one entry", v["annotations"])
	}
	a := annots[0].(map[string]any)
	if a["note"] != "spike" || a["span"] != "1s" {
		t.Errorf("annotation = %v, want note=spike span=1s", a)
	}
}

func TestBytes(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0B"}, {512, "512B"}, {1024, "1.0KiB"},
		{1536, "1.5KiB"}, {2 << 30, "2.0GiB"},
	}
	for _, tt := range tests {
		if got := Bytes(tt.in); got != tt.want {
			t.Errorf("Bytes(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestElapsed(t *testing.T) {
	if got := Elapsed(90 * time.Second); got != "1:30" {
		t.Errorf("Elapsed = %q, want 1:30", got)
	}
	if got := Elapsed(3661 * time.Second); got != "1:01:01" {
		t.Errorf("Elapsed = %q, want 1:01:01", got)
	}
}
