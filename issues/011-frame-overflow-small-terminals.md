# 011 — Frame overflow on small terminals

Status: open

## Motivation

The frame height is unbounded below the graphs: the cursor info line and
the annotation list grow with every annotation. On a small terminal
(rows < graphs + notes + chrome) the frame exceeds the screen and the
alt screen scrolls, breaking the `ESC[H` repaint layout.

## Sketch

- `frame()` already receives `rows`: clamp the annotation list to the
  remaining space, ending with a spec-labeled overflow line
  ("… +3 more").
- Graph height already shrinks for small terminals; extend that budget
  to account for info/annotation lines.
