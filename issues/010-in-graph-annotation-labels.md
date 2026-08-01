# 010 — In-graph annotation labels with arrows

Status: done (2026-08-01)

Implemented as specced below. The marker overlays bars when needed (only
the text requires blank space — mock-up style, arrows touch the data);
labels auto-fall-through CPU → MEM rows and are skipped when nothing fits.
Marker glyph and max label width are spec-defined (`graph.marker`,
`graph.label_width`, notes truncated with `…`). Verified in a pty: label
placed in the graph's blank area, pointing at the column, traveling with
the graph after resume, included in screenshots.

## Motivation

Mock-up: `~/git/ffext/lazypins/screens/004-mem-usage-firefox-annotated.png`
— annotation text drawn inside the graph area with an arrow pointing at
the spot ("opening tabs" → bar, "unload other tabs" ↓ column), instead of
only a list below the graphs.

## Behavior

- Each visible annotation renders its note **inside a graph**: a marker
  glyph (`▼`, spec-defined) sits at the annotated column, the note text
  beside it (left or right, wherever blank graph space allows), colored
  distinctly from the bars.
- Placement is automatic: try blank runs on the top rows of the CPU then
  MEM graph; skip the label when nothing fits (column highlight + note
  list below still identify it).
- Labels move with the graph after resume (timestamps, as before) and are
  included in screenshots.
- The note list below the graphs stays (full text + span values); the
  in-graph label may truncate long notes.

## Sketch

- spec: `graph.marker` glyph; reuse `label`/`annot` theme tokens.
- tui: place labels on the raw rune grids before colorization
  (`placeLabels`), then a `renderRow` pass merges overlay cells, column
  highlights, and base graph color.
