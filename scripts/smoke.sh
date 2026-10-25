#!/usr/bin/env bash
# ============================================================================
# scripts/smoke.sh — end-to-end checks against the real Telegram Bot API.
#
# Needs TG_BOT_TOKEN and TG_TEST_CHAT (a chat the bot can write to, ideally
# your DM with it). Every message it sends is silent and deleted at the end.
# Fails if any output ever contains the token. Works with bash 3.2 (macOS).
#
#   make smoke                    # builds ./bin/tg first
#   TG=./bin/tg scripts/smoke.sh
#   TG_API_BASE=http://localhost:8081 scripts/smoke.sh   # local Bot API server
#
# In GitHub Actions it also writes passed/total/failed to $GITHUB_OUTPUT
# (the notify job reports them to Telegram).
# ============================================================================
set -euo pipefail

: "${TG_BOT_TOKEN:?smoke requiere TG_BOT_TOKEN}"
: "${TG_TEST_CHAT:?smoke requiere TG_TEST_CHAT}"
TG="${TG:-./bin/tg}"
CHAT="$TG_TEST_CHAT"

# tg CMD ARGS...: the binary under test, with --api-base right after the
# command when TG_API_BASE is set (flags must precede positional text).
tg() {
  local cmd="$1"; shift
  "$TG" "$cmd" ${TG_API_BASE:+--api-base "$TG_API_BASE"} "$@"
}

OUT="$(mktemp)"
# Isolated config dir: the no-token check must not find a real profile.
EMPTY_XDG="$(mktemp -d)"
IDS=()
DELETED=""
PASS=0
FAILED=""
TOTAL="$(grep -c '^run "' "$0")"
cleanup() {
  local id
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    { echo "passed=$PASS"; echo "total=$TOTAL"; echo "failed=$FAILED"; } >>"$GITHUB_OUTPUT"
  fi
  # ${a[@]+...}: an empty array is "unbound" under set -u in bash < 4.4.
  for id in ${IDS[@]+"${IDS[@]}"}; do
    [[ -n "$id" && "$id" != "$DELETED" ]] || continue
    tg delete --chat "$CHAT" --message-id "$id" -q >/dev/null 2>&1 || true
  done
  rm -rf "$OUT" "$EMPTY_XDG"
}
trap cleanup EXIT

fail() { FAILED="$*"; echo "FAIL: $*" >&2; [[ -s "$OUT" ]] && sed 's/^/    /' "$OUT" >&2; exit 1; }

# Anything shaped like a bot token (real or the fake one below); the
# redacted form "123456789:***" does not match.
TOKEN_RE='[0-9]{6,}:[A-Za-z0-9_-]{30,}'

# run NAME WANT_CODE CMD...: runs CMD capturing stdout+stderr into $OUT,
# checks the exit code and that no token ever shows up. Paced to stay under
# Telegram's ~1 msg/s per chat (each tg call is a fresh process, so the
# client's own limiter can't pace across them).
run() {
  local name="$1" want="$2"; shift 2
  local code=0
  [[ "$PASS" -eq 0 ]] || sleep "${TG_SMOKE_PACE:-1}"
  "$@" >"$OUT" 2>&1 || code=$?
  if grep -qF "$TG_BOT_TOKEN" "$OUT" || grep -qE "$TOKEN_RE" "$OUT"; then
    : >"$OUT"  # don't echo the leak into CI logs
    fail "$name: el token apareció en la salida"
  fi
  [[ "$code" == "$want" ]] || fail "$name: exit $code, esperado $want"
  PASS=$((PASS + 1))
  echo "ok   $name"
}

# ids_from_out: every message_id=N printed by the last run.
ids_from_out() { grep -oE 'message_id=[0-9]+' "$OUT" | cut -d= -f2 || true; }
# add_ids: remember them for cleanup.
add_ids() { local x; while read -r x; do if [[ -n "$x" ]]; then IDS+=("$x"); fi; done < <(ids_from_out); }

sha="${GITHUB_SHA:-local}"
stamp="tg smoke $(date -u +%Y-%m-%dT%H:%M:%SZ) ${sha:0:7}"

run "version" 0 "$TG" version
run "me" 0 tg me
grep -q '^ok id=' "$OUT" || fail "me: salida inesperada"
run "me --json" 0 tg me --json
grep -q '"is_bot":true' "$OUT" || fail "me --json: falta is_bot"

run "send" 0 tg send --silent --chat "$CHAT" "$stamp"
id="$(ids_from_out)"; [[ "$id" =~ ^[0-9]+$ ]] || fail "send: sin message_id"; IDS+=("$id")

run "send -q" 0 tg send -q --silent --chat "$CHAT" "$stamp (quiet)"
qid="$(tr -d '[:space:]' <"$OUT")"; [[ "$qid" =~ ^[0-9]+$ ]] || fail "send -q: '$qid' no es un id"; IDS+=("$qid")

run "send stdin + --html-escape" 0 bash -c 'printf "a <b> & c\n" | "$0" send ${2:+--api-base "$2"} --silent --html-escape --chat "$1" -' "$TG" "$CHAT" "${TG_API_BASE:-}"
add_ids

run "send HTML" 0 tg send --silent --chat "$CHAT" "<b>$stamp</b> <code>html</code>"
add_ids

run "send --split" 0 tg send --silent --split --chat "$CHAT" "$(printf 'x%.0s' $(seq 1 5000))"
n="$(ids_from_out | wc -l)"
[[ "$n" -ge 2 ]] || fail "send --split: esperadas ≥2 partes, hubo $n"
add_ids

run "send --document" 0 tg send --silent --chat "$CHAT" --document ./go.mod --caption "$stamp (doc)"
add_ids

run "edit" 0 tg edit --chat "$CHAT" --message-id "$id" "$stamp (editado)"
run "react" 0 tg react --chat "$CHAT" --message-id "$id" --emoji "👍"
run "copy" 0 tg copy --chat "$CHAT" --from "$CHAT" --message-id "$id"
add_ids
run "action" 0 tg action --chat "$CHAT" --action typing
run "chat" 0 tg chat --chat "$CHAT"

# Failure paths keep their exit codes.
run "texto demasiado largo sin --split → 2" 2 tg send --chat "$CHAT" "$(printf 'x%.0s' $(seq 1 5000))"
run "HTML inválido → 4" 4 tg send --silent --chat "$CHAT" "<b>sin cerrar"
run "chat inexistente → 4" 4 tg send --silent --chat 1 "x"
run "sin token → 3" 3 env -u TG_BOT_TOKEN -u TELEGRAM_BOT_TOKEN XDG_CONFIG_HOME="$EMPTY_XDG" TG="$TG" TG_API_BASE="${TG_API_BASE:-}" \
  bash -c '"$TG" me ${TG_API_BASE:+--api-base "$TG_API_BASE"}'
# A well-formed but revoked/wrong token: Telegram answers 401 → exit 4.
run "token incorrecto → 4" 4 env TG_BOT_TOKEN=123456789:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA TG="$TG" TG_API_BASE="${TG_API_BASE:-}" \
  bash -c '"$TG" me ${TG_API_BASE:+--api-base "$TG_API_BASE"}'


run "delete" 0 tg delete --chat "$CHAT" --message-id "$id"
DELETED="$id"  # cleanup skips it

echo "smoke: $PASS/$TOTAL checks OK"
