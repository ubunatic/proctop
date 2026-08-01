# 009 — Annotations + screenshot in pause mode

Status: done (2026-08-01)

Annotations half implemented: in pause mode `←/→` (or `h/l`) move a column
cursor (with a `▶ time cpu mem` info line), `v` marks a range anchor, `a`
opens a single-line note — Enter saves, Esc cancels (structural keys;
Ctrl-C still quits). Annotations are **pegged to sample timestamps**, never
column indexes: highlights travel with the graph after resume and scroll
off naturally while the note list (`● time–end cpu mem — note`, with span
maxima as values) and the summary JSON (`annotations: [{time, span,
note}]`) keep them. Pause snapshots the history so cursor targets stay
stable while sampling continues underneath. Cursor/selection/annotation
column highlights are spec-themed background colors; screenshots include
highlights and note list. Covered by handleKey-level unit tests and a
live pty run.

Screenshot half implemented: `s`/`S` in pause mode saves the current frame
(minus the hint line) as `.txt` (colors stripped) **and** `.ansi` (raw,
`cat`-viewable) — formats, filename template (`proctop-<target>-<ts>`),
and time layout are spec-defined; existing files are never overwritten
(`-2`, `-3`, … suffix). A green `📷 saved <files>` notice appears in the
hint line, cleared on resume. The TUI was refactored into a pure
`frame() []string` + `paint()` split so screenshots and terminal output
share one renderer and frames are unit-testable.

Remaining: the annotations part below (cursor, area select, notes,
annotations embedded in screenshots and summary JSON).

## Motivation

Once the display can be paused (issue 008), you want to mark interesting
regions ("this spike = tab load") and save the annotated view as evidence.

## Behavior

- Only in pause mode:
  - move a column cursor (arrow keys) over the graph; select a column or
    a column range (area) and attach a short text note,
  - annotated columns/areas are highlighted; notes listed below the graph
    with the sample's timestamp and values.
- `[s]` saves a screenshot **including annotations** (spec-defined key).

## Sketch

- Screenshot = the rendered frame, saved as:
  - `.txt` (plain, colors stripped) and/or `.ansi` (raw escapes; viewable
    with `cat`) — decide format(s); maybe both via spec default.
  - Filename pattern spec-defined, e.g. `proctop-<target>-<ts>.txt`.
- Annotations keyed by sample timestamp (not column index) so they stay
  correct as history scrolls after resume; persist them into the summary
  JSON (`annotations: [{time, span, note}]`) so they survive the TUI.
- Spec: new keys (`annotate`, `screenshot`, cursor movement), labels, and
  highlight color token; cursor keys land in a `hidden_keys`-style group.
- Keep MVP scope: single-line notes, no editing UI beyond enter/esc.
