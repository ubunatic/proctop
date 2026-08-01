# 003 — CSV export + user-defined line templates

Status: open

## Motivation

JSON lines are machine-readable but spreadsheet users want CSV, and some
pipelines want custom columns.

## Sketch

- `--format csv` with a header row on new/empty files (spec-defined columns).
- `--template '...'` flag overriding the spec line template for one run.
- Spec: add `formats.csv` (column list) to `config.yaml` + schema.
