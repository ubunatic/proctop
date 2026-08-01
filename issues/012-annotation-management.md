# 012 — Manage annotations: delete, edit, persist across runs

Status: open

## Motivation

Annotations are append-only (MVP scope of [009](009-pause-annotations-screenshot.md)):
a typo or misplaced span cannot be fixed, and annotations die with the
process — a watch session cannot be resumed with its notes.

## Sketch

- Pause mode: with the cursor on an annotated column, `d` deletes and
  `a` edits (prefills the draft) the covering annotation — spec keys.
- Optional `--annotations FILE` to load/save annotations as JSON (same
  shape as the summary's `annotations` field), enabling re-runs against
  a recorded session's notes.
