Adhere to the following conventions.

## Project Summary

`proctop` is a Go CLI that watches CPU/MEM usage of one process and its
process tree over time (live TUI graph or plain/color/JSON line stream),
tracks peaks, and prints/writes a summary on exit. Key components live in
`internal/` packages; user-facing text, colors, keys, and templates live
in `spec/` (see Spec system below).

## Development Scripts

Run from project root. `make help` lists all targets
(build, test, vet, validate-spec, check, install, clean).

<!-- harnez:begin Language Conventions -->
Adhere to the following conventions.

Docs in `./docs/` are managed by harnez. <!-- harnez:bundled -->

- Go/Golang @docs/Go.md,
  Modern Go, avoid deps but use Cobra, add tests
- Make/Makefile @docs/Make.md,
  ⚙️ phony sentinel, self-doc help, build dependency pattern
- Markdown @docs/Markdown.md,
  PascalCase for evergreens, kebab-case for ephemeral docs; ASCII art in chat, Mermaid only in docs/
- Git @docs/Git.md,
  conventional commits, work on the default branch, don't push unless asked
- Canary-first development @docs/Canary.md,
  probe external mechanisms before building features on them
- Spec system @docs/Spec.md,
  YAML spec files as single source of truth; Go code must not duplicate spec values
- Agentic Loop Practices @docs/AgenticLoop.md,
  5-phase loop (Advisory -> Dev -> Review -> Hygiene -> Retro), zero zombie guarantee
- Issue Tracking Practices @docs/IssueTracking.md,
  P0-P3 priorities, metadata headers (Status, Priority, Severity, Category), tracker sync
<!-- harnez:end Language Conventions -->

## Project Rules

- `spec/config.yaml` (+ `spec/schemas/config.schema.json`) is the single
  source of truth for labels, theme colors, key bindings, line/summary
  templates, and defaults. It is embedded via `spec/spec.go` and guarded
  by `go test ./spec/...` (`make validate-spec`). When adding user-facing
  text or a key binding: update the YAML, the schema, and the spec tests —
  never hardcode it in Go. Ctrl-C stays hardcoded as last-resort quit.
- Planned features are tracked as `issues/NNN-kebab-case.md` files with a
  `Status:` line; keep `issues/README.md` in sync.
- Evergreen docs live in `docs/`: read `docs/Design.md` before changing
  sampling math, target resolution, or the spec layout; `docs/Testing.md`
  explains the pty harness for TUI verification.
