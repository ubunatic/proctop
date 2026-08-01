// Package format renders samples and summaries as plain, colored, or JSON
// lines. Line and summary layouts come from the spec templates; this package
// only supplies the data and the color template funcs.
package format

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	"codeberg.org/ubunatic/proctop/internal/sampler"
	"codeberg.org/ubunatic/proctop/spec"
)

// Kinds supported by New. The default comes from spec defaults.format.
const (
	Plain = "plain"
	Color = "color"
	JSON  = "json"
)

// LineFunc renders one sample as a single output line (no newline).
type LineFunc func(sampler.Sample) (string, error)

// lineData is the template context for spec line templates.
type lineData struct {
	Time  string
	CPU   string
	Mem   string
	Procs int
}

// jsonLine is the machine-readable per-sample record.
type jsonLine struct {
	Time     time.Time `json:"time"`
	CPUPct   float64   `json:"cpu_pct"`
	MemBytes int64     `json:"mem_bytes"`
	Procs    int       `json:"procs"`
}

// Funcs returns the template FuncMap for spec templates: fg emits the ANSI
// sequence for a theme token, off resets.
func Funcs(cfg *spec.Config) template.FuncMap {
	return template.FuncMap{
		"fg": func(token string) (string, error) {
			n, ok := cfg.Theme[token]
			if !ok {
				return "", fmt.Errorf("format: unknown theme token %q", token)
			}
			return Fg(n), nil
		},
		"off": func() string { return Off },
	}
}

// Fg returns the ANSI 256-color foreground sequence for palette index n.
func Fg(n uint8) string { return fmt.Sprintf("\x1b[38;5;%dm", n) }

// Bg returns the ANSI 256-color background sequence for palette index n.
func Bg(n uint8) string { return fmt.Sprintf("\x1b[48;5;%dm", n) }

// Off is the ANSI reset sequence.
const Off = "\x1b[0m"

// New returns a LineFunc for the given kind (plain, color, json).
func New(kind string, cfg *spec.Config) (LineFunc, error) {
	switch kind {
	case JSON:
		return func(s sampler.Sample) (string, error) {
			b, err := json.Marshal(jsonLine{s.Time, s.CPUPct, s.RSS, s.Procs})
			return string(b), err
		}, nil
	case Plain, Color:
		text := cfg.Formats.Plain
		if kind == Color {
			text = cfg.Formats.Color
		}
		tpl, err := template.New(kind).Funcs(Funcs(cfg)).Parse(text)
		if err != nil {
			return nil, fmt.Errorf("format: parse %s template: %w", kind, err)
		}
		return func(s sampler.Sample) (string, error) {
			var sb strings.Builder
			err := tpl.Execute(&sb, lineData{
				Time:  s.Time.Format(cfg.Formats.Time),
				CPU:   Pct(s.CPUPct),
				Mem:   Bytes(float64(s.RSS)),
				Procs: s.Procs,
			})
			return sb.String(), err
		}, nil
	}
	return nil, fmt.Errorf("format: unknown format %q", kind)
}

// summaryData is the template context for the spec summary template.
type summaryData struct {
	Target     string
	RootPID    int
	Start, End string
	Elapsed    string
	Samples    int
	CPUMax     string
	CPUMaxTime string
	CPUMin     string
	CPUAvg     string
	MemMax     string
	MemMaxTime string
	MemMin     string
	MemAvg     string
}

// jsonSummary is the machine-readable summary record.
type jsonSummary struct {
	Target      string           `json:"target"`
	RootPID     int              `json:"root_pid"`
	Start       time.Time        `json:"start"`
	End         time.Time        `json:"end"`
	Samples     int              `json:"samples"`
	CPUMaxPct   float64          `json:"cpu_max_pct"`
	CPUMaxTime  time.Time        `json:"cpu_max_time"`
	CPUMinPct   float64          `json:"cpu_min_pct"`
	CPUAvgPct   float64          `json:"cpu_avg_pct"`
	MemMaxBytes int64            `json:"mem_max_bytes"`
	MemMaxTime  time.Time        `json:"mem_max_time"`
	MemMinBytes int64            `json:"mem_min_bytes"`
	MemAvgBytes int64            `json:"mem_avg_bytes"`
	Annotations []jsonAnnotation `json:"annotations,omitempty"`
}

// jsonAnnotation is one user note in the summary record.
type jsonAnnotation struct {
	Time time.Time `json:"time"`
	Span string    `json:"span"`
	Note string    `json:"note"`
}

// SummaryText renders the human-readable exit summary from the spec template.
func SummaryText(cfg *spec.Config, target string, rootPID int, r *sampler.Recorder) (string, error) {
	tpl, err := template.New("summary").Funcs(Funcs(cfg)).Parse(cfg.Summary.Text)
	if err != nil {
		return "", fmt.Errorf("format: parse summary template: %w", err)
	}
	clock := cfg.Formats.Time
	var sb strings.Builder
	err = tpl.Execute(&sb, summaryData{
		Target:     target,
		RootPID:    rootPID,
		Start:      r.Start.Format(clock),
		End:        r.End.Format(clock),
		Elapsed:    Elapsed(r.End.Sub(r.Start)),
		Samples:    r.Samples,
		CPUMax:     Pct(r.CPUMax.Value),
		CPUMaxTime: r.CPUMax.Time.Format(clock),
		CPUMin:     Pct(r.CPUMin.Value),
		CPUAvg:     Pct(r.CPUAvg()),
		MemMax:     Bytes(r.MemMax.Value),
		MemMaxTime: r.MemMax.Time.Format(clock),
		MemMin:     Bytes(r.MemMin.Value),
		MemAvg:     Bytes(r.MemAvg()),
	})
	return sb.String(), err
}

// SummaryJSON renders the summary as a single JSON line.
func SummaryJSON(target string, rootPID int, r *sampler.Recorder) (string, error) {
	var annots []jsonAnnotation
	for _, a := range r.Annotations {
		annots = append(annots, jsonAnnotation{
			Time: a.Start,
			Span: a.End.Sub(a.Start).String(),
			Note: a.Note,
		})
	}
	b, err := json.Marshal(jsonSummary{
		Target:      target,
		RootPID:     rootPID,
		Start:       r.Start,
		End:         r.End,
		Samples:     r.Samples,
		CPUMaxPct:   r.CPUMax.Value,
		CPUMaxTime:  r.CPUMax.Time,
		CPUMinPct:   r.CPUMin.Value,
		CPUAvgPct:   r.CPUAvg(),
		MemMaxBytes: int64(r.MemMax.Value),
		MemMaxTime:  r.MemMax.Time,
		MemMinBytes: int64(r.MemMin.Value),
		MemAvgBytes: int64(r.MemAvg()),
		Annotations: annots,
	})
	return string(b), err
}

// Pct formats a percentage value with one decimal, without the %% sign.
func Pct(v float64) string { return fmt.Sprintf("%.1f", v) }

// Bytes formats a byte count as a human-readable IEC size.
func Bytes(v float64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f%s", v, units[i])
	}
	return fmt.Sprintf("%.1f%s", v, units[i])
}

// Elapsed formats a duration as h:mm:ss or m:ss.
func Elapsed(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
