// Package spec embeds the YAML spec files and their JSON schemas into the
// binary and loads them into typed structs. The spec is application code:
// Go code must not duplicate or shadow values defined here.
package spec

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"

	"gopkg.in/yaml.v3"
)

//go:embed *.yaml schemas
var root embed.FS

// FS is the embedded spec filesystem. Paths are relative to the spec/
// directory, e.g. fs.ReadFile(spec.FS, "config.yaml").
var FS fs.FS = root

// Config mirrors spec/config.yaml.
type Config struct {
	App struct {
		Name  string `yaml:"name"`
		Title string `yaml:"title"`
	} `yaml:"app"`
	Defaults struct {
		Interval    string `yaml:"interval"`
		History     int    `yaml:"history"`
		GraphHeight int    `yaml:"graph_height"`
		Format      string `yaml:"format"`
	} `yaml:"defaults"`
	Keys struct {
		Quit  []string `yaml:"quit"`
		Pause []string `yaml:"pause"`
	} `yaml:"keys"`
	Theme map[string]uint8 `yaml:"theme"`
	Graph struct {
		Levels string `yaml:"levels"`
	} `yaml:"graph"`
	Labels  map[string]string `yaml:"labels"`
	Formats struct {
		Plain string `yaml:"plain"`
		Color string `yaml:"color"`
		Time  string `yaml:"time"`
	} `yaml:"formats"`
	Summary struct {
		Text string `yaml:"text"`
	} `yaml:"summary"`
}

// Load parses the embedded config.yaml. Unknown YAML keys are an error so
// that spec and Go structs cannot drift silently.
func Load() (*Config, error) {
	data, err := fs.ReadFile(FS, "config.yaml")
	if err != nil {
		return nil, fmt.Errorf("spec: %w", err)
	}
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("spec: parse config.yaml: %w", err)
	}
	return &cfg, nil
}

// ResolveKey maps a named key alias from the spec to its terminal byte
// sequence. Literal single-character keys pass through unchanged.
func ResolveKey(name string) string {
	switch name {
	case "<esc>":
		return "\x1b"
	case "<c-c>":
		return "\x03"
	case "<cr>":
		return "\x0d"
	case "<space>":
		return " "
	case "<bs>":
		return "\x7f"
	}
	return name
}
