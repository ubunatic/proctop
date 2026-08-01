# proctop

`proctop` watches **one process and its process tree** (all children,
recursively) and records CPU and memory usage over time — as a live TUI
graph or as a plain/colored/JSON line stream. Peaks are tracked and a
summary is printed on exit.

```sh
proctop firefox            # live TUI graph (q to quit)
proctop 4242               # watch by PID
proctop -f plain firefox   # stream one line per second
proctop -f json  firefox   # JSON lines for machine consumption
proctop -o usage.log -s summary.json firefox   # export samples + summary
```

## Modes

- **TUI** (default on a terminal): current CPU/MEM, running min/max with
  timestamps, and block-character history graphs. `p` (or space) pauses
  the display to inspect a spike — sampling and export keep running, so
  resuming fast-forwards with no data gap. While paused: `←/→` move a
  column cursor, `v` marks a range, `a` attaches a note (Enter saves,
  Esc cancels) — annotations are pegged to sample timestamps, stay
  highlighted as the graph moves on, are listed below the graphs, and
  land in the summary JSON. `s` saves the frame as `.txt` and `.ansi`
  screenshots (`proctop-<target>-<ts>.*`, never overwriting), including
  annotations. Quit with `q` — the exit summary is printed to the normal
  screen.
- **Stream** (`--stream`, any `--format`, or piped stdout): one line per
  interval in `plain`, `color`, or `json` format.

In both modes `--out FILE` appends per-sample lines (colors are stripped
for files written from the TUI) and `--summary FILE` appends a one-line
JSON summary on exit.

## Flags

| Flag | Default | Purpose |
|---|---|---|
| `-i, --interval` | `1s` | sampling interval |
| `-f, --format` | `color` | `plain` \| `color` \| `json` (implies `--stream`) |
| `-o, --out` | — | append per-sample lines to file |
| `-s, --summary` | — | append JSON summary to file on exit |
| `-n, --history` | `300` | samples kept for graph/history |
| `--stream` | off | line stream instead of TUI |

## How it works

- Sampled from `/proc/<pid>/stat` once per interval: `utime+stime` tick
  deltas become CPU% (of one core, summed over the tree — may exceed 100%),
  RSS pages become bytes.
- The tree is re-discovered every tick, so newly spawned children are
  picked up and dead ones dropped. When the root process exits, proctop
  exits with the summary.
- A name target picks the topmost matching process with the largest
  subtree, so launcher stubs lose against the real main process.

## Code layout

- `spec/` — **application code as data**: labels, colors, keys, output
  templates, defaults (`config.yaml` + JSON schema, embedded into the
  binary). Go code must not duplicate spec values; spec integrity is
  guarded by `go test ./spec/...`.
- `internal/proc` — `/proc` scanning, stat parsing, tree discovery
- `internal/sampler` — time series, peak/min/avg tracking
- `internal/format` — plain/color/JSON line + summary rendering
- `internal/tui` — live dashboard (raw mode, alt screen, block graphs)
- `internal/app` — run loop wiring both modes
- `docs/` — design decisions and testing guide ([index](docs/README.md))
- `issues/` — planned features ([index](issues/README.md))

Build with `make build`, test with `make check`.

## License

AGPL-3.0-or-later — see `LICENSES/`. REUSE compliant (`make reuse`).
