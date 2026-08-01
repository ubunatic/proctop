# 005 — Run duration limit and threshold alerts

Status: open

## Motivation

Unattended benchmarking runs need to stop by themselves and flag
regressions without a human watching the graph.

## Sketch

- `--duration 5m` stops sampling and prints the summary.
- `--max-cpu 200 --max-mem 4GiB`: exit non-zero (and mark the summary)
  when a threshold is exceeded — usable in CI.
- Threshold hits highlighted in the TUI using the spec `max` color.
