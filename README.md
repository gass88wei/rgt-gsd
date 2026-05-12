# rgt-gsd

AI agent orchestration with version control.

**rgt-gsd** combines:
- [re_gent](https://github.com/vovsdkhl/re_gent) — version control for AI agent activity (blame, log, rewind)
- [gsd-2](https://github.com/gass88wei/gsd-2) — autonomous agent execution engine (plan, dispatch, recover)

## Quick start

```bash
# Prerequisites
npm install -g gsd-pi          # gsd-2 CLI
go install github.com/regent-vcs/regent/cmd/rgt@latest  # re_gent CLI

# Install rgt-gsd
go install github.com/gass88wei/rgt-gsd/cmd/rgt-gsd@latest

# Initialize a project
rgt-gsd init

# Run a gsd-2 plan with automatic audit trail
rgt-gsd run
```

## Commands

```
rgt-gsd run              Execute plan with audit + recovery
rgt-gsd init             Initialize project
rgt-gsd audit blame      Trace code to prompt
rgt-gsd audit log        View session history
rgt-gsd exec status      Check gsd-2 status
rgt-gsd exec stop        Stop execution
rgt-gsd health           Check workspace and tools
```

## Architecture

Four sub-agents orchestrated by a pipeline:

```
pipeline
├── workspace    git state, conflict detection, environment prep
├── auditor      version control (blame, log, rewind, diff)
├── executor     gsd-2 execution (spawn, monitor, resume)
└── recovery     failure analysis, rewind, retry logic
```
