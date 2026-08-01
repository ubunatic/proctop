# proctop Testing

How the project is tested and how to exercise the TUI without a human at
a terminal.

## Layers

| Layer | Where | What |
|---|---|---|
| Unit | `internal/*/..._test.go` | stat parsing, root heuristic, CPU math, peaks, graph shapes, formatters |
| Spec integrity | `spec/spec_test.go` (`make validate-spec`) | spec loads strictly, templates parse, all theme tokens/labels/keys used by Go exist, graph levels = 9 runes |
| Live smoke | manual / pty harness | stream mode against a real process, TUI frames in a pty |

Everything runs with `make check`. There are no integration tests for
`internal/app` yet — [issue 007](../issues/007-app-integration-tests.md).

## Testing the TUI in a pty

This host has **no `script` binary**, so use Python's `pty` module. This
harness starts proctop in a pty, sets a real window size, quits with `q`
after a few seconds, and dumps the raw escape stream:

```python
import os, pty, time, sys, select, fcntl, struct, termios
pid, fd = pty.fork()
if pid == 0:
    os.execv("./proctop", ["proctop", "firefox"])
fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 20, 90, 0, 0))
os.set_blocking(fd, False)
out, end, sent = b"", time.time() + 5, False
while time.time() < end + 2:
    if not sent and time.time() > end:
        os.write(fd, b"q"); sent = True
    r, _, _ = select.select([fd], [], [], 0.2)
    if r:
        try: data = os.read(fd, 65536)
        except OSError: break
        if not data: break
        out += data
os.close(fd)
sys.stdout.buffer.write(out)
```

Inspect frames with `sed 's/\x1b/\n<ESC>/g'`, or extract the last frame by
splitting on `ESC[H` and stripping SGR sequences.

**Set the winsize.** A fresh pty reports 0×0; `term.GetSize` returns
those zeros *without an error*. This exact gap made all graph rows vanish
during development (renderer computed width −2) and is now guarded in
`internal/tui`, but any new size-dependent code must handle non-positive
sizes.

## Interaction tests without a pty

`view.handleKey(key) (quit, dirty)` holds all TUI interaction, so key
flows are plain unit tests: resolve spec keys with `spec.ResolveKey`,
feed them through `handleKey`, assert on the view/recorder state (see
`TestAnnotateFlow` — pause → cursor → mark → type → commit → resume).
Reserve the pty harness for what units can't see: escape-sequence
output, alt-screen behavior, and visual layout.

## Pitfalls seen in this repo's own verification

- `timeout N ./proctop --stream ... | head` can show **nothing at all**
  (pipe teardown artifact) and look like an app bug. Redirect to a file
  first (`> out.txt`), then inspect — stream mode was working all along.
- Verify name-target resolution against reality: `pgrep` + `/proc/<pid>/
  cmdline` exposed the Firefox dbus stub that broke the naive heuristic
  (see [Design.md](Design.md), target resolution).
- Unit tests pass ≠ visible on screen. The in-graph labels shipped green
  but never appeared against real Firefox data: flat full-height bars
  occupied every marker cell, and the placement rule "marker cell must be
  blank" silently skipped every label. A second visual-only bug —
  connector colored like the highlight background — was spotted by the
  user in real use. Always follow rendering changes with a pty run
  against live data and *look at the frame*.

## Related

- [Design.md](Design.md) — the decisions these tests protect
