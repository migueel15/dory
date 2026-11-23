---
name: dory-workflow
description: "How to work on a task with Dory (MCP server `dory`): register it, plan it, read and save memories, hand it over and close it with few calls. Use for any feature, fix or task in a project that Dory tracks, or to resume one."
version: 3
---

# Dory workflow

Dory tracks projects, tasks, plans and memories, and the user follows them in its interface. Keep that state in Dory, never in files. Every Dory call must answer something the next step needs; never call Dory just to load context.

## Model

| Entity | What it is |
|---|---|
| Task | A unit of work with branch, issue and pull request. `stage` is `inbox`, `ready`, `done` or `cancelled`; `description` holds the goal, the scope and how to test it. |
| Column | The kanban state. Dory derives it from threads and the pull request; never set it. |
| Plan | One Markdown document per task. Write it with `plan_write`, change it with `plan_edit`. Only the user accepts it. |
| Memory | A durable decision or lesson. `scope` is `project`, `global` or `node`; `kind` is `decision`, `lesson` or `context`; it has a short `title`. |

If the work needs a state or field that does not exist, ask the user; do not encode it some other way.

## 1. Start with what you have

- In a Dory thread, the session instructions carry a card with the thread, folder, branch, project, repository and linked task. Trust it; do not call `session_start`, `list_tasks` or `recall` to start.
- Outside a Dory thread, or without a project in the card, call `session_start` once with the repository path. It returns an index, not contents.

## 2. Register the work

1. If the card shows a task, that is the task. Call `get_task` only if the user's message does not already say what to do.
2. Otherwise `search` with two or three key words. Found: `ensure_thread_task(task_id)`. Not found: `ensure_thread_task(title, description)`. Never create a task without searching first.
3. Write a plan only when the task needs one, and only after `ensure_thread_task`.

## 3. Ask the user

When a decision belongs to the user, ask with `ask_user` from the Dory MCP: one to four questions, each with a short header and two to four options; the user can always answer freely. The thread waits for the answer. Do not leave questions as loose text at the end of a turn.

When you need a password, sudo password, token, API key, PIN or 2FA code, ask with `request_secret`, never in the chat. You get file paths, not values: read a file only inside the command that uses it (`sudo -S -p '' cmd < "$file"`, `tool --token "$(cat "$file")"`) and never print it.

## 4. Memory: read on demand, write rarely

- Read with `recall(query, project_id)` only before a design decision, when you touch an area with known history, or when something behaves unexpectedly. `recall(id)` reads one memory in full.
- Save with `remember(kind, title, content)` only a `decision` the user agreed that outlasts the task, or a `lesson` you verified that the code and its history do not show.
- Do not save what the code, the task, the plan or the pull request already record: no progress, file lists or commit summaries. Keep content short.
- The same title in the same scope updates that memory. Fix a stale one with `remember(id, ...)`.

## 5. Write when the work changes state

Use the answer of each write as its confirmation; do not read it back.

| Moment | Call |
|---|---|
| Scope agreed or changed | `save_task(id, description)` |
| Issue created | `save_task(id, issue_number)` |
| Blocked by another task | `add_task_dependency` |
| Abandoned | `save_task(id, stage: "cancelled")` |

Dory links the pull request by the task branch; set `pr_number` only when the branch differs.

## 6. Hand over and close

- When the work is finished, make sure the task description says how to test it and tell the user. The column moves with the pull request.
- If the user asks for an independent check, start one subagent with the `single-test-run` skill.
- A task with a pull request becomes done when the pull request is merged; never mark it yourself. A task without one: `save_task(id, stage: "done")` once the user confirms.

## Other Dory tools

- Long-running processes (dev servers, watchers): `start_service`, `stop_service`.
- Passwords, tokens and other private values: `request_secret`.
- Charts, diagrams, tables or mockups shown in the chat: check the page with `html_preview`, then show it with `render_html`.
- Subagents: `subagent_spawn`; do not poll, the child's end wakes you.
- The shared browser panel: `browser_*`. Android devices: `device_*`.
