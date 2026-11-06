---
name: dory-dev-preview
description: Start the Dory development environment from the current branch or worktree so the user can try a finished feature at http://infra-admin:5173 before merging. Use only in the Dory repository, when an item is ready for the user to test (moving to validating), or when the user asks to preview, stop or check the dev environment.
---

# Dory dev preview

Only for work on the Dory project itself, on `infra-admin`, where production Dory runs.

## When

- Right after the work on a Dory item is finished and pushed, before or as you move the item to `validating`.
- When the user asks to preview a branch, or to stop or check the dev environment.

## How

From the root of the checkout or worktree that holds the branch:

```sh
scripts/dev-preview.sh start    # stops any open Dory dev environment, then starts this branch
scripts/dev-preview.sh status   # shows the branch and the URL
scripts/dev-preview.sh stop     # stops it
```

- `start` copies production data into `dory_dev`, runs migrations and serves the branch with Vite (HMR) and air, in the user systemd unit `dory-dev`. It returns once the page answers, or prints the unit log if it fails.
- There is only one dev environment. `start` replaces the previous one, including one launched by hand with `scripts/dev.sh`. Production (`:8080`, database `dory`) is never touched.
- It needs `air` (`go install github.com/air-verse/air@latest`) and runs `npm install` in `web/` when `node_modules` is missing.
- Logs: `journalctl --user -u dory-dev -f`.

## After starting

- Tell the user the URL (`http://infra-admin:5173`) and the branch, so they can try it from any device at home or over Tailscale. The UI shows a `dev` badge next to "Dory".
- Mention in the item `progress` that the branch is live in the dev environment.
- Leave it running. It stops when another branch starts, when the user asks, or by itself within a minute after the branch pull request is merged (into any base). Merging is the validation: Dory then moves the item to `done` on its own.
- If `start` fails, report the log lines and record the failure in the item `progress`; do not loop on retries.
