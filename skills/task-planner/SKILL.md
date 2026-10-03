---
name: task-planner
description: Record an agent's substantive work plan, actual progress, and verification in the shared task-planner vault. Use for multi-step work or artifact changes, across all projects; skip simple answers and one-step read-only checks.
---

# Task Planner

Track work the agent actually undertakes in the human's shared `tp` vault (`$TP_VAULT` when set, otherwise `~/tasks`). Never create a per-project or agent sub-vault. Set `executor: agent` on every task the agent performs; tasks without the field are human tasks. Use `project` when an existing project fits the work, and name the repository/path in the note.

## Before work

After enough inspection to state a concrete plan, search the shared vault with `executor:agent` for an open task for this same objective. Continue it when found, including after session restarts; don't duplicate it for each message or session. If absent, create one before substantive edits or external actions. The initial note should state the requested outcome, target path, 2–5 meaningful steps, and what would count as completion. Mark it `doing` when work begins. Record only intended work as a plan; never imply a step already happened.

Use the configured task-planner MCP when available. Call `vault_info` before the first write and require `mode: shared` and the expected base vault path. Then use `task_query`, `task_add` with `executor: agent`, `note`, and `start: true`, followed by `task_note`, `task_status`, and `task_get`. If MCP is unavailable or points elsewhere, use the local `tp` CLI with the base vault:

```sh
tp list 'executor:agent status:doing'
tp add '작업 제목' --executor agent --start --note '목표: ...
대상: ...
계획: 1. ... 2. ...
완료 기준: ...'
```

Use `--vault "$TP_VAULT"` when the configured base differs from the default. Keep the returned full task ID; short numbers can be ambiguous in legacy data or migration history.

## Taking assigned work

A task may name who should run it (`agent: claude|codex|auto`) and how heavy it is (`tier: fast|standard|deep`). `vault_info` returns the allowed agents, the tier→model table, and this connection's session id.

- To find your share, query `pick:<your agent>` (named for you or `auto`, open, not on hold, unclaimed). Do not take tasks named for another agent, and do not sweep up unnamed tasks; those are the human's.
- Before working on a pickable task, call `task_claim` (empty `ref` takes the most urgent one). It refuses a task another run already claimed: report that instead of working around it. CLI: `tp claim [ref] --agent <agent> --session <id>`.
- Use the returned `model` for the work, for example as the subagent model. If it is empty, judge the tier yourself, record it with `task_edit` (`tier`), and note the reason.
- If you stop without finishing, call `task_release`. Never force-release another run's claim unless the user confirms that run has ended.

## During work

Append a dated note after a meaningful step, a changed plan, a blocker, or a validation result. Say what actually happened and include the relevant file, command outcome, error, or artifact reference. Keep notes brief; the task is a progress record, while code and documents remain the source for current implementation. Do not write speculative success, sensitive values, or a transcript of every tool call. If the user changes scope, update the active task's plan in a note before continuing.

At the end, record the delivered result and verification, then set `done` only when the requested work is complete. Use `blocked` with the specific dependency if work cannot continue, or `cancelled` if the user abandons it. If the session ends unexpectedly, an old `doing` state proves only that no closing update was recorded: inspect the artifacts and resume or correct its status. If tp is unavailable, continue authorized work and tell the user which record could not be written.

Never modify task Markdown directly. The user can inspect the same vault with `tp` (TUI), press `F` to switch human → agent → all, or run `tp list 'executor:agent'`.
