# From process tree to annotated timeline: proctop

A process monitor usually answers one narrow question: what is using resources right now? `proctop` was built to answer a more useful debugging question: how did one application and all of its child processes behave over time? It watches CPU and memory for a process tree, then presents the measurements as a terminal graph or a stream that another tool can consume.

The code is a Go CLI. It reads Linux `/proc` process statistics, follows descendants, computes CPU from tick deltas, and sums resident memory. Go packages divide process discovery, sampling, formatting, terminal rendering, and app wiring. Cobra provides the command interface; `x/term` handles terminal mode; YAML configuration, checked against a JSON schema and embedded in the binary, supplies labels, colors, keys, defaults, and output templates. The output formats are plain text, colored text, and JSON lines. There is no external service or network protocol in the monitoring path.

The first hard design problem was choosing the right process when a name matches more than one root. During development, Firefox exposed a childless `--dbus-service` stub with the same name as the browser. A naive lowest-PID match selected a one-process, 14 MiB tree instead of the real browser tree with 18 processes and about 2.5 GiB. The implementation now selects the topmost matching process with the largest subtree, using the lowest PID to break ties. The decision and its limits are in `docs/Design.md`; interactive selection remains a tracked feature request (issue 002).

The terminal view grew into an investigation surface. Pause freezes the display while sampling and export continue. The user can move a cursor through history, select a time range, attach a note, and save the frame as text or ANSI. Notes are attached to sample timestamps, so they follow the underlying measurement as the graph advances. In-graph labels place a marker on the measurement and use available blank space for the note. This design required more than drawing a graph: the renderer splits frame construction from terminal painting so the same frame can serve unit tests and screenshots. Key behavior is held in `handleKey`, where pause, cursor, selection, annotation, and screenshot flows can be exercised without terminal automation.

A concrete rendering bug showed why that separation was not enough by itself. In live Firefox data, full-height bars occupied the cells where the label-placement rule expected a blank marker position, so labels disappeared even though unit tests passed. The rule was changed: label text still needs room, but the marker and connector can overlay the graph bars. A second visual defect made the connector disappear against the highlighted annotation area because it reused the highlight color. The connector received its own spec color. The project records these findings in its testing guide and calls for pseudo-terminal checks after visual changes.

The history shows an individual contributor-sized project whose agent harness makes decisions, checks, and follow-up work persistent. `issues/` holds numbered planned features; completed tickets 008–014 record behaviors and verification episodes. The repository includes Harnez-generated development conventions and release integration, a GoReleaser configuration, and a `make release` path through Harnez. The docs describe multi-agent sprint and review patterns as workspace practice, but the available project history does not establish that those workflows were used to build these features. The most instructive agentic practice visible here is the durable loop: record a bug or decision, implement against the spec, write tests, verify the live terminal behavior, then update the issue and design notes. The latest six-week window is quiet, with four commits (August 27 to September 5, 2026), largely release setup and managed documentation synchronization; the larger feature run happened earlier.

At the time of this review, the repository has 3,127 lines of Go, including 1,122 test lines, and 45 named Go test functions. `go test ./...` passes. The remaining issues cover a process picker, richer metrics, app-level integration tests, small-terminal overflow, and annotation editing/persistence. That makes the project a useful example of agents helping a developer build a cohesive, testable CLI with a deliberately narrow Linux interface, while keeping unresolved limits visible rather than presenting the MVP as finished.

## Facts

| Item | Detail |
|---|---|
| Stack | Go 1.26.5, Cobra, `x/term`, YAML, JSON schema, Linux `/proc`, ANSI terminal |
| Repository size | 3,127 Go lines; 1,122 test lines; 45 named Go tests |
| Recent activity | 4 commits, 2026-08-27 through 2026-09-05; quiet since |
| Release | v0.1.0, 2026-08-29 (`a3d0363`) |
| Key feature tickets | Issues 008–014, marked done |
