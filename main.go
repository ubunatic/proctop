// Command proctop watches CPU and memory usage of a single process and its
// process tree over time: as a live TUI graph or as a plain/colored/JSON
// line stream, with peak tracking and an exit summary.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"codeberg.org/ubunatic/proctop/internal/app"
	"codeberg.org/ubunatic/proctop/spec"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cfg, err := spec.Load()
	if err != nil {
		// The embedded spec is part of the binary; failing to load it is a build defect.
		panic(err)
	}
	defInterval, err := time.ParseDuration(cfg.Defaults.Interval)
	if err != nil {
		panic(fmt.Errorf("spec: defaults.interval: %w", err))
	}

	var (
		interval    time.Duration
		formatKind  string
		outPath     string
		summaryPath string
		history     int
		stream      bool
	)
	cmd := &cobra.Command{
		Use:     cfg.App.Name + " [flags] <name|pid>",
		Version: Version,
		Short:   "watch CPU/MEM of a process tree over time",
		Long: cfg.App.Name + ` watches one process and all its children (by name or PID)
and records CPU%% and RSS once per interval.

Default is a live TUI graph. With --stream (or when stdout is not a
terminal) it prints one line per sample instead. Peaks are tracked and a
summary is printed on exit.`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			streamMode := stream || cmd.Flags().Changed("format") || !app.IsTTY()
			a, err := app.New(cfg, app.Options{
				Target:      args[0],
				Interval:    interval,
				Format:      formatKind,
				StreamMode:  streamMode,
				OutPath:     outPath,
				SummaryPath: summaryPath,
				History:     history,
			})
			if err != nil {
				return err
			}
			return a.Run()
		},
	}
	cmd.Flags().DurationVarP(&interval, "interval", "i", defInterval, "sampling interval")
	cmd.Flags().StringVarP(&formatKind, "format", "f", cfg.Defaults.Format, "line format: plain|color|json (implies --stream)")
	cmd.Flags().StringVarP(&outPath, "out", "o", "", "append per-sample lines to this file")
	cmd.Flags().StringVarP(&summaryPath, "summary", "s", "", "append JSON summary to this file on exit")
	cmd.Flags().IntVarP(&history, "history", "n", cfg.Defaults.History, "samples kept for graph and history")
	cmd.Flags().BoolVar(&stream, "stream", false, "print sample lines instead of the TUI")
	return cmd
}
