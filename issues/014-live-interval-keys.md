# 014 — Adjust the sampling interval live with [+]/[-]

Status: done (2026-08-01)

Implemented as specced below: spec-defined `+`/`-` keys walk the spec
`interval_steps` ladder, the ticker resets immediately, and the title
readout follows. Verified in a pty (1s → 2s → 4s → 2s → 1s, also while
paused, and floor-and-back retraces the same steps) and by unit tests on
`handleKey`.

First cut doubled/halved with min/max clamping; that broke symmetry at
the bounds (1s → … → 100ms floor → back up landed on 1.6s, not 1s), so
it was replaced by the fixed step ladder.

## Motivation

The `--interval` flag is fixed at start. Watching a live graph you often
want to slow sampling down (long-running watch) or speed it up (zoom into
a burst) without restarting and losing the recording.

## Behavior

- `+` steps the sampling interval up the spec `defaults.interval_steps`
  ladder (100ms … 1m in 1-2-5 steps), `-` steps it down (spec
  `interval_up`/`interval_down` keys). Works in play and pause mode; the
  play-mode hint line shows a spec-labeled `[+-] interval`.
- `+`/`-` are always symmetric: walking past a ladder end and back
  retraces the same steps. An off-ladder `--interval` start value snaps
  to the next step in the pressed direction and stays on the ladder.
- The title readout (`interval 2s`) updates immediately; the sampling
  ticker resets so the new rate takes effect at once.
- Recording is unaffected: CPU% math uses real sample timestamps, so
  mixed-interval history stays correct (peaks, averages, export lines).

## Sketch

- tui: `view.adjustInterval(up bool)` picks the next larger/smaller
  `interval_steps` entry; `Run` compares the interval around `handleKey`
  and calls `ticker.Reset` on change.
- Spec: new keys `interval_up: ["+"]`, `interval_down: ["-"]`, label
  `hint_interval`, defaults `interval_steps` (yaml + schema + struct +
  tests; the spec test enforces a strictly ascending ladder containing
  the default interval).
