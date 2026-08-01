# 008 — TUI pause/play without losing samples

Status: open

## Motivation

Watching a live graph, you want to freeze the display to inspect a spike —
without a gap in the recording.

## Behavior

- `[p]` toggles pause/play (spec-defined key + hint label).
- **Pause freezes rendering only**: the sampler keeps ticking in the
  background, history/peaks/export keep recording every sample.
- On play, the display fast-forwards to the current state — the graph
  jumps ahead, no records lost, no gap in `--out` files.
- Paused state is clearly visible (e.g. spec-labeled `⏸ paused` marker in
  the title line, `max` color).

## Sketch

- tui: a `paused` flag skips `render()` on tick but still calls `tick()`
  and `onSample`. Keys: add `pause` action to `spec/config.yaml` keys +
  schema + labels; keep Ctrl-C hardcoded.
- Recorder needs no change (it already records independent of rendering).
- Groundwork for annotations in [issue 009](009-pause-annotations-screenshot.md).
