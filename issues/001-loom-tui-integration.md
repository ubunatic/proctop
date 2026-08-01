# 001 — Port the TUI to loom widgets

Status: deferred — loom is not mature yet (owner decision, 2026-08-01);
keep the self-contained ANSI TUI until loom stabilizes.

## Motivation

The MVP TUI is a self-contained ANSI renderer. The workspace standard for
TUIs is `codeberg.org/ubunatic/loom` (shared look & feel with uman/uzu/cati).
loom's `Pane.Run` event loop is currently input-blocking with no tick/timer
events, so a 1 Hz live dashboard cannot be driven through it yet.

## Sketch

- Add a tick/timer event source to loom's event loop (upstream change),
  or drive a loom `Canvas` from proctop's own loop and only reuse
  `Canvas`/`Style` for rendering.
- Reuse loom key decoding (`DecodeKey`) instead of raw byte matching.
- Keep the spec-driven keys/theme; map spec theme tokens to `loom.Style`.
