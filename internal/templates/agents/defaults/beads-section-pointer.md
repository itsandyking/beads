## Beads Issue Tracker

This project tracks work in **bd (beads)**: a dependency-aware issue graph that outlives
any single session, machine, or agent. Run `bd prime` for the workflow reference and this
project's stored memories.

```bash
bd ready                # unblocked work
bd show <id>            # issue detail
bd update <id> --claim  # take it atomically (safe when agents run concurrently)
bd close <id>           # finish
bd create "Title" -p 1  # file work; add --deps discovered-from:<id> for work found mid-task
```

Reach for bd when work outlives this conversation, has blockers, or another agent may
touch it — a session-scoped todo list cannot hold any of those. Scratch tracking that
dies with the conversation can stay wherever your host already keeps it.

Issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote.
`.beads/issues.jsonl` is a passive export, not the sync protocol.
