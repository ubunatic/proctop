# 007 — Integration tests for internal/app and the TUI

Status: open

## Motivation

`internal/app` (mode wiring, export/summary files, exit paths) has no
tests, and TUI verification is manual. The pty harness that caught the
0×0-winsize bug (see [docs/Testing.md](../docs/Testing.md)) should run in CI.

## Sketch

- app: run a short stream session against `os.Getpid()` (always exists),
  assert sample lines, `--out` append, and `--summary` JSON shape.
- TUI smoke test: Go equivalent of the Python pty harness
  (`creack/pty` would be a new dep — or keep the Python script under
  `make smoke`), assert alt-screen enter/leave, graph rows present at a
  known winsize, `q` exits with a summary.
- Wire into `make check` once stable.
