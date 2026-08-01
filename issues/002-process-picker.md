# 002 — Interactive process picker for ambiguous names

Status: open

## Motivation

A name target can match several process trees (e.g. two browser profiles,
launcher stubs). The MVP silently picks the root with the largest subtree.
Show the candidates and let the user pick instead.

## Sketch

- When multiple root candidates match, list them (pid, comm, #procs, RSS)
  and prompt for a choice; `--first` keeps the current auto-pick behavior.
- Natural fit for `loom.Choice` once [001](001-loom-tui-integration.md) lands.
- Non-interactive mode (piped stdout) keeps auto-pick.
