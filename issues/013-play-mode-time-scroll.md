# 013 — Scroll through time in play mode

Status: done (2026-08-01)

Implemented as specced below: `←`/`→` pan the live graph through recorded
history, spec-labeled `⏪ history -m:ss` marker with the lag behind the
live edge, anchored view while new samples arrive, and cursor auto-scroll
in pause mode. Verified in a pty (marker appears, lag grows while
anchored) and by unit tests on `handleKey`/`frame`.

## Motivation

The recorder keeps more history (default 300 samples) than fits on
screen, but the live view only ever showed the newest window. A spike
that scrolled off the left edge was unreachable without pausing first —
and even paused, the cursor could not walk past the visible edge.

## Behavior

- In play mode, `←`/`→` (the spec `cursor_left`/`cursor_right` keys)
  scroll the graph window back/forward through the recorded history.
- While scrolled back, the view stays **anchored to the samples it
  shows**: new samples keep recording but do not shift the view. The
  title line shows a spec-labeled `⏪ history -0:42` marker (lag behind
  the live edge) and the play-mode hint line shows `[←→] scroll`.
- Scrolling fully right returns to the live edge; the marker disappears
  and the graph follows new samples again.
- Scrolling left clamps when the oldest full screen is visible.
- Pausing while scrolled keeps the position; the cursor starts at the
  visible right edge. Resuming play fast-forwards to the live edge
  (scroll resets), consistent with [issue 008](008-tui-pause-play.md).
- In pause mode the cursor now auto-scrolls the window, so it can walk
  through the whole frozen history instead of leaving the screen.

## Sketch

- tui: `view.scroll` counts samples hidden right of the view (0 = live).
  `frame` truncates the history slice by the clamped scroll before the
  existing window/graph/marks logic; each new live sample increments a
  non-zero scroll to keep the anchor (`view.advance`).
- `view.gwidth` remembers the graph width of the last frame so key
  handling clamps against what is actually displayed.
- Spec: new labels `hint_scroll` and `scrolled` (yaml + schema + tests);
  no new keys — play mode reuses `cursor_left`/`cursor_right`.
