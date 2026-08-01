# 006 — More metrics: IO, threads, PSS

Status: open

## Motivation

CPU/RSS is the MVP. Disk IO, thread counts, and PSS (accurate shared-memory
accounting) round out the picture for heavy multi-process apps.

## Sketch

- IO: `/proc/<pid>/io` read_bytes/write_bytes deltas (needs same-user or root).
- Threads: `num_threads` is already in `/proc/<pid>/stat` (field 20).
- PSS: sum `/proc/<pid>/smaps_rollup` (slower — sample less often or opt-in).
- Extra graphs/rows in the TUI behind spec-defined labels and colors.
