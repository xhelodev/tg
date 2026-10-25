---
name: tg
description: Working in the xhelodev/tg repo (Go CLI for the Telegram Bot API). Use before writing code, running builds/tests, or opening PRs here.
---

# tg — repo workflow for coding agents

Compact workflow guide. For API/reference details read [AGENTS.md](../../AGENTS.md);
for human-facing docs read [README.md](../../README.md).

## Before writing any code — hard constraints

1. **Zero external dependencies.** `go.mod` is stdlib-only. Never add third-party modules.
2. **Never print the bot token.** Not to stdout, stderr, logs, or error JSON — in any code path.
3. **Static builds only.** Every build/install uses `CGO_ENABLED=0`.
4. **Exit codes are a contract:** `0` ok · `1` bug · `2` usage · `3` config/token · `4` API `ok:false` · `5` network · `130` interrupted.
5. **Library purity:** `pkg/telegram` never calls `os.Exit` and never prints; only `internal/cli` does I/O and exits.
6. User-facing messages are written in **Spanish** (existing convention); code identifiers and comments in English.

## Build & test (run before every PR)

```bash
make test          # go test ./...
make vet           # go vet ./...
make build         # CGO_ENABLED=0 go build ./cmd/tg
```

All three must pass. New features need table-driven tests next to the code
(see `internal/cli/*_test.go`, `pkg/telegram/*_test.go` for the style —
no external assertion libraries). If your change touches `install.sh`,
smoke-test it with `bash -n` plus a `--dry-run` and a real run against a
temp `HOME`.

## PR workflow

1. Branch from `main`: `feat/…`, `fix/…`, or `docs/…`.
2. Conventional commits (`feat:`, `fix:`, `docs:`, `refactor:`), one logical
   change per PR. Include the *why* in the body, not just the what.
3. Push and open with `gh pr create --fill-first`.
4. **An automated reviewer (pr-reviewer bot, Hermes/glm-5.3) comments within
   ~15 minutes.** Fix everything under "Críticos" and push to the same branch —
   it re-reviews the new SHA automatically. "Advertencias" are judged case by
   case; leave a comment if you consciously skip one.
5. Squash-friendly history: don't pile unrelated fixes into a review branch.

## Never do this

- Commit tokens, `.env` files, or anything from `~/.config/tg/`.
- Force-push `main` or merge without the review bot's verdict.
- Edit `AGENTS.md`, `README.md`, or `llms.txt` casually — keep them in sync
  with real behavior; they are load-bearing for other agents.
- Bump the Bot API version claim without updating the affected methods.

## Quick layout

```
cmd/tg/          # thin main -> cli.Run
pkg/telegram/    # typed client (pure library)
internal/cli/    # flags, commands, token resolve, output, exit codes
internal/config/ # ~/.config/tg/config.json (XDG)
install.sh       # readable installer: clone/pull -> go build -> PATH
```
