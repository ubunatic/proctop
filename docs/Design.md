# proctop Design

Architecture and the reasoning behind the MVP (2026-08-01). Read this
before changing sampling math, target resolution, or the spec layout.

## Data flow

```
/proc/<pid>/stat  ──►  internal/proc      scan all pids, parse stat,
                        │                  build tree (root + descendants)
                        ▼
                       internal/sampler   tick deltas → CPU%, RSS sum,
                        │                  bounded history, min/max/avg
            ┌───────────┴───────────┐
            ▼                       ▼
     internal/format         internal/tui       spec/config.yaml
     plain/color/json        raw ANSI dashboard  (labels, theme, keys,
     lines + summary         block graphs         templates — embedded)
            │                       │
            ▼                       ▼
     stdout / --out file     alt screen; summary on exit
```

`internal/app` wires it together: one `tick()` (scan → tree → record) is
shared by both modes; mode selection lives in `main.go` (TUI on a TTY,
stream when `--stream`, `--format`, or piped).

## Key decisions

### Spec-driven UI (workspace convention)
All labels, theme colors, key bindings, line/summary templates, graph
characters, and defaults live in `spec/config.yaml` with a JSON schema,
embedded via `go:embed`. Go never duplicates spec values; the strict
decoder (`KnownFields`) plus `spec/spec_test.go` catch drift in both
directions. Ctrl-C stays hardcoded in the TUI as last-resort quit,
independent of spec keys.

### Self-contained ANSI TUI, no loom
loom (`codeberg.org/ubunatic/loom`) is the workspace TUI library, but its
`Pane.Run` event loop is input-blocking with no tick/timer events — a 1 Hz
dashboard cannot be driven through it — and the owner considers loom not
mature yet. The TUI is therefore ~250 lines of raw ANSI (alt screen, raw
mode via `x/term`, full repaint per tick with `ESC[K`/`ESC[J` clearing).
Port tracked in [issue 001](../issues/001-loom-tui-integration.md)
(deferred).

### Target resolution: largest subtree wins
A name target can match several topmost processes. Real-world pitfall
found during development: Firefox has a childless `--dbus-service` stub
named `firefox`, and "lowest PID wins" picked it (1 proc, 14 MiB) instead
of the real browser (18 procs, 2.5 GiB). `proc.FindRoot` therefore picks
the topmost match with the **largest process tree**, tie-broken by lowest
PID. Interactive disambiguation is
[issue 002](../issues/002-process-picker.md).

### CPU% math
CPU% = Σ per-PID `(utime+stime)` tick deltas / CLK_TCK / Δt × 100, percent
of one core summed over the tree (may exceed 100, like `top`). Properties:

- The first sample only primes counters (`Record` returns `ok=false`).
- A newly appeared child contributes **no** CPU on its first tick (no
  previous value to delta against) — deliberate, keeps spikes honest.
- A dead child's remaining ticks are lost for its final interval —
  accepted MVP inaccuracy.
- CLK_TCK is read from the ELF auxiliary vector (`/proc/self/auxv`,
  `AT_CLKTCK`), falling back to 100. Go has no sysconf; auxv avoids
  shelling out to `getconf`.

### Memory metric
RSS from `/proc/<pid>/stat` field 24 × page size, summed over the tree.
This double-counts shared pages across processes — fine for trend
watching, not for absolute accounting. PSS via `smaps_rollup` is
[issue 006](../issues/006-more-metrics-io-threads.md).

### Tree discovery: full /proc rescan per tick
Every tick re-reads all of `/proc/*/stat` and rebuilds the child map,
instead of using `/proc/<pid>/task/*/children` (which needs
`CONFIG_PROC_CHILDREN` and misses reparented processes). A full scan at
1 Hz is cheap, picks up newly spawned children, and drops dead ones.
When the root itself exits, proctop exits with the summary.

### /proc/stat parsing
`comm` is enclosed in parens and may contain spaces **and parens**
(e.g. `(Web Content (x))`), so fields are split only after the *last*
`)`. Man-page field N maps to `fields[N-3]` after that split — see
`parseStat` and its test.

## Rendering notes

- Graphs use 1/8-block characters (`▁▂▃▄▅▆▇█` from the spec) — each
  column has `height × 8` sublevels, scaled to the visible-window max.
  Tiny non-zero values are forced to one visible sublevel.
- `term.GetSize` can return `0×0 without an error` on a fresh pty; the
  renderer treats non-positive sizes as unavailable and falls back to
  80×24 (found via the pty test harness — see [Testing.md](Testing.md)).
- Export files never receive ANSI colors from TUI mode (`color` downgrades
  to `plain` for `--out`).

## Related

- [Testing.md](Testing.md) — test layers and the pty harness
- [issues/](../issues/README.md) — planned features and status
