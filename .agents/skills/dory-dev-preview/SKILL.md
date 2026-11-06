---
name: dory-dev-preview
description: Start, stop or check the Dory development environment for the current branch or worktree, served at http://infra-admin:5173. Only when the user explicitly asks for it; never start it on your own when a task finishes.
disable-model-invocation: true
---

# Dory dev preview

Only for the Dory repository, on `infra-admin`, where production Dory runs. Run it only when the user asks; the user decides when the dev environment is up.

## How

From the root of the checkout or worktree that holds the branch:

```sh
scripts/dev-preview.sh start    # stops any open Dory dev environment, then starts this branch
scripts/dev-preview.sh status   # shows the branch and the URL
scripts/dev-preview.sh stop     # stops it
```

- `start` copies production data into `dory_dev`, runs migrations and serves the branch with Vite (HMR) and air, in the user systemd unit `dory-dev`. It returns once the page answers, or prints the unit log if it fails.
- There is only one dev environment. `start` replaces the previous one, including one launched by hand with `scripts/dev.sh`; say so if another branch was live. Production (`:8080`, database `dory`) is never touched.
- It stops when another branch starts, on `stop`, or by itself within a minute after the branch pull request is merged (into any base).
- It needs `air` (`go install github.com/air-verse/air@latest`) and runs `npm install` in `web/` when `node_modules` is missing.
- Logs: `journalctl --user -u dory-dev -f`.

## After running it

- Tell the user the result: the URL (`http://infra-admin:5173`) and the branch after `start`, or that it is stopped.
- If `start` fails, report the log lines; do not retry on your own.
