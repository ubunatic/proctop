# Inital Prompt
```
see ../projects and create a Go app here that:
  - shows CPU/MEM usage over time for a single process / process tree
    - use firefox as example, which is currently running
  - app must show CPU MEM as time series
    - one row per second
    - plain text, colored text, json fmt
    - export as file (append)
    - show as graph TUI
  - focus on watching one process + its tree and spawned children
  - record peek usage
  - write peek as summary file
    - or print summary when TUI exits
    - print peek/min in live TUI too

  - make sure key part of the app code stay in ./spec/yaml + schemas
  - make sure key components are defined in Go packages

  Overall:
  - keep it simple
  - focus on MVP
  - plan features as issues/ MD files to pick up later
```
