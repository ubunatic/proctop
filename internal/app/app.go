// Package app wires proc scanning, sampling, formatting, and output modes
// (stream or TUI) into the proctop run loop.
package app

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/term"

	"codeberg.org/ubunatic/proctop/internal/format"
	"codeberg.org/ubunatic/proctop/internal/proc"
	"codeberg.org/ubunatic/proctop/internal/sampler"
	"codeberg.org/ubunatic/proctop/internal/tui"
	"codeberg.org/ubunatic/proctop/spec"
)

// Options are the fully resolved run options (flag values with spec defaults
// already applied by the CLI layer).
type Options struct {
	Target      string        // process name or PID
	Interval    time.Duration // sampling interval
	Format      string        // plain | color | json
	StreamMode  bool          // stream lines instead of TUI
	OutPath     string        // append per-sample lines to this file ("" = off)
	SummaryPath string        // write JSON summary to this file ("" = off)
	History     int           // graph/history length in samples
}

// App holds the run state for one watch session.
type App struct {
	cfg     *spec.Config
	opts    Options
	rec     *sampler.Recorder
	rootPID int
	line    format.LineFunc
	outFile *os.File
}

// New resolves the target process and prepares the recorder and formatters.
func New(cfg *spec.Config, opts Options) (*App, error) {
	all, err := proc.ReadAll()
	if err != nil {
		return nil, err
	}
	rootPID, err := proc.FindRoot(all, opts.Target)
	if err != nil {
		return nil, err
	}
	lineKind := opts.Format
	if opts.OutPath != "" && !opts.StreamMode {
		// Never write ANSI colors into the export file from TUI mode.
		if lineKind == format.Color {
			lineKind = format.Plain
		}
	}
	line, err := format.New(lineKind, cfg)
	if err != nil {
		return nil, err
	}
	a := &App{
		cfg:     cfg,
		opts:    opts,
		rec:     sampler.New(proc.ClockTicks(), opts.History),
		rootPID: rootPID,
		line:    line,
	}
	if opts.OutPath != "" {
		f, err := os.OpenFile(opts.OutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, fmt.Errorf("app: open export file: %w", err)
		}
		a.outFile = f
	}
	return a, nil
}

// Run executes the watch loop in the selected mode and always finishes by
// printing the summary and writing the summary file if requested.
func (a *App) Run() error {
	defer func() {
		if a.outFile != nil {
			a.outFile.Close()
		}
	}()
	var runErr error
	if a.opts.StreamMode {
		runErr = a.runStream()
	} else {
		runErr = tui.Run(a.cfg, a.opts.Interval, a.rec,
			tui.Meta{Target: a.opts.Target, RootPID: a.rootPID}, a.tick, a.export)
	}
	if err := a.finish(); err != nil && runErr == nil {
		runErr = err
	}
	return runErr
}

// tick performs one scan+record step. It fails when the watched tree is gone.
func (a *App) tick() (sampler.Sample, bool, error) {
	all, err := proc.ReadAll()
	if err != nil {
		return sampler.Sample{}, false, err
	}
	tree := proc.Tree(all, a.rootPID)
	if len(tree) == 0 {
		return sampler.Sample{}, false, fmt.Errorf("app: process %d exited", a.rootPID)
	}
	s, ok := a.rec.Record(time.Now(), tree)
	return s, ok, nil
}

// export appends one formatted line to the export file, if configured.
func (a *App) export(s sampler.Sample) {
	if a.outFile == nil {
		return
	}
	line, err := a.line(s)
	if err == nil {
		fmt.Fprintln(a.outFile, line)
	}
}

// runStream prints one line per interval until interrupted or the tree exits.
func (a *App) runStream() error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	ticker := time.NewTicker(a.opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return nil
		case <-ticker.C:
			s, ok, err := a.tick()
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			line, err := a.line(s)
			if err != nil {
				return err
			}
			fmt.Println(line)
			a.export(s)
		}
	}
}

// finish prints the human summary and writes the JSON summary file.
func (a *App) finish() error {
	if a.rec.Samples == 0 {
		return nil
	}
	text, err := format.SummaryText(a.cfg, a.opts.Target, a.rootPID, a.rec)
	if err != nil {
		return err
	}
	fmt.Print(text)
	if a.opts.SummaryPath != "" {
		js, err := format.SummaryJSON(a.opts.Target, a.rootPID, a.rec)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(a.opts.SummaryPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return fmt.Errorf("app: open summary file: %w", err)
		}
		defer f.Close()
		if _, err := fmt.Fprintln(f, js); err != nil {
			return fmt.Errorf("app: write summary: %w", err)
		}
	}
	return nil
}

// IsTTY reports whether stdout is a terminal (used to pick the default mode).
func IsTTY() bool { return term.IsTerminal(int(os.Stdout.Fd())) }
