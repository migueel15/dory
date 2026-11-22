---
name: single-test-run
description: "Test a finished Dory task exactly once against what the task says should work, and report without fixing anything. Use only when asked for an independent check of a finished task."
version: 1
---

# Single test run

Run one test pass and report. Do not fix, refactor, retry or start other agents.

## Input

The brief gives the Dory task id, the repository and the branch. Read the task with `get_task`: its `description` says the goal and how to test it, its plan the agreed design, and the card the branch and pull request. If how to test is missing, test the stated goal and say so.

## Process

1. Check out or inspect the branch without changing files.
2. Run the checks once: the project's tests and linters that cover the change, plus each behaviour the task lists. Read the code when a behaviour cannot be run.
3. Mark what needs a person (interface, hardware, external accounts) as `pending_human`, with concrete steps.

## Report

Answer in the user's language:

- Result: pass or fail.
- One line per check: what was checked, `pass`, `fail`, `not_run` or `pending_human`, and the evidence (command and relevant output, or file and line).
- For each failure: what fails, how to reproduce it, and the likely place in the code.

Do not write to Dory; the agent that asked records the result. Do not modify files, commit or push.
