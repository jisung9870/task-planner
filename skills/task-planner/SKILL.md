---
name: task-planner
description: Record an agent's substantive work plan, actual progress, and verification in the shared task-planner agent vault. Use for multi-step work or artifact changes, across all projects; skip simple answers and one-step read-only checks.
---

# Task Planner

Track work the agent actually undertakes in one shared `tp` vault. The human's default vault is the base path (`$TP_VAULT` when set, otherwise `~/tasks`). The agent vault is exactly `<base>/agent`, regardless of working directory or project. Never create a per-project vault or write agent work to the human vault. Use the task's note to identify the repository/path; leave `project` unset unless the user explicitly asks to use it.

## Before work

After enough inspection to state a concrete plan, search the agent vault for an open task for this same objective. Continue it when found, including after session restarts; don't duplicate it for each message or session. If absent, create one before substantive edits or external actions. The initial note should state the requested outcome, target path, 2–5 meaningful steps, and what would count as completion. Mark it `doing` when work begins. Record only intended work as a plan; never imply a step already happened.

Use the configured task-planner MCP when available. Call `vault_info` before the first write and require `mode: agent` and the expected `<base>/agent` path. Then use `task_query`, `task_add` (with `note` and `start: true`), `task_note`, `task_status`, and `task_get`. If MCP is unavailable or points elsewhere, use the local `tp` CLI with an explicit agent vault path. For example, with the default base:

```sh
tp --vault "$HOME/tasks/agent" init
tp --vault "$HOME/tasks/agent" list 'status:doing'
tp --vault "$HOME/tasks/agent" add '작업 제목' --start --note '목표: ...
대상: ...
계획: 1. ... 2. ...
완료 기준: ...'
```

When `$TP_VAULT` is set, append `/agent` to its value instead of `$HOME/tasks`. Run `init` only if the agent vault does not yet exist. Keep the returned full task ID for later calls; short numbers are vault-local and can be ambiguous in legacy data.

## During work

Append a dated note after a meaningful step, a changed plan, a blocker, or a validation result. Say what actually happened and include the relevant file, command outcome, error, or artifact reference. Keep notes brief; the task is a progress record, while code and documents remain the source for current implementation. Do not write speculative success, sensitive values, or a transcript of every tool call. If the user changes scope, update the active task's plan in a note before continuing.

At the end, record the delivered result and verification, then set `done` only when the requested work is complete. Use `blocked` with the specific dependency if work cannot continue, or `cancelled` if the user abandons it. If the session ends unexpectedly, an old `doing` state proves only that no closing update was recorded: inspect the artifacts and resume or correct its status. If tp is unavailable, continue authorized work and tell the user which record could not be written.

`tp note <id> "<fact>"`, `tp done <id>`, `tp block <id> "<reason>"`, and `tp cancel <id>` all accept `--vault <base>/agent` before the command. Never modify task Markdown directly. The user can inspect the same vault with `tp --vault <base>/agent` (TUI) or `list`/`show`.
