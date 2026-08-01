# 004 — Per-process breakdown view

Status: open

## Motivation

The tree total hides which child eats the CPU/MEM (e.g. one Firefox tab
process). A breakdown makes the tool useful for diagnosing runaway children.

## Sketch

- TUI: toggle key (spec-defined) switching to a top-N children table
  (pid, comm, CPU%, RSS), sorted by CPU.
- Stream: `--per-process` emits one JSON object per process per tick.
- sampler: keep per-PID deltas instead of only the tree sum.
