#!/usr/bin/env bash
# ============================================================================
# scripts/ci-notify.sh — sends one Telegram message with the CI summary.
#
# Called by the `notify` job in .github/workflows/ci.yml with the results of
# the other jobs in the environment. Silent on success, with sound on
# failure. Plain text (no parse mode): PR titles and branch names may hold
# characters that would break HTML.
#
# Env: TG (binary), TG_TEST_CHAT, TEST_RESULT, SCRIPTS_RESULT, SMOKE_RESULT,
#      UNIT_PASSED, UNIT_TOTAL, SMOKE_PASSED, SMOKE_TOTAL, SMOKE_FAILED,
#      PR_NUMBER, PR_TITLE, REF_NAME, SHA, RUN_URL  (all optional but TG_TEST_CHAT)
#      DRY_RUN=1 prints the message instead of sending it.
# ============================================================================
set -euo pipefail

: "${TG_TEST_CHAT:?ci-notify requiere TG_TEST_CHAT}"
TG="${TG:-./bin/tg}"

# icon RESULT: GitHub's needs.<job>.result → emoji.
icon() {
  case "$1" in
    success) echo "✅" ;;
    failure) echo "❌" ;;
    cancelled) echo "🚫" ;;
    skipped | "") echo "⏭️" ;;
    *) echo "❔" ;;
  esac
}

# Anything but success/skipped (failure, cancelled, startup_failure…) is a failure.
ok=1
for r in "${TEST_RESULT:-}" "${SCRIPTS_RESULT:-}" "${SMOKE_RESULT:-}"; do
  case "$r" in success | skipped) ;; *) ok=0 ;; esac
done

unit="$(icon "${TEST_RESULT:-}")"
if [[ -n "${UNIT_TOTAL:-}" ]]; then
  unit+=" ${UNIT_PASSED:-0}/${UNIT_TOTAL}"
fi

smoke="$(icon "${SMOKE_RESULT:-}")"
if [[ -n "${SMOKE_TOTAL:-}" && -n "${SMOKE_PASSED:-}" ]]; then
  smoke+=" ${SMOKE_PASSED}/${SMOKE_TOTAL}"
fi
if [[ -n "${SMOKE_FAILED:-}" ]]; then
  smoke+=" — falló: ${SMOKE_FAILED}"
elif [[ -z "${SMOKE_TOTAL:-}" ]]; then
  case "${SMOKE_RESULT:-}" in
    # No outputs: skipped, or "succeeded" without running any check.
    success | skipped | "") smoke="⏭️ (no se ejecutó)" ;;
    *) smoke+=" (sin resultados: falló antes de los checks)" ;;
  esac
fi

if [[ -n "${PR_NUMBER:-}" ]]; then
  where="PR #${PR_NUMBER}: ${PR_TITLE:-}"
  detail="rama ${REF_NAME:-?}${SHA:+ · ${SHA:0:7}}"
else
  where="${REF_NAME:-?}"
  detail="${SHA:+commit ${SHA:0:7}}"
fi

if [[ "$ok" -eq 1 ]]; then
  head="✅ tg CI OK"
else
  head="❌ tg CI falló"
fi

msg="${head} · ${where}${detail:+
${detail}}

unit tests: ${unit}
scripts: $(icon "${SCRIPTS_RESULT:-}")
smoke (Telegram real): ${smoke}
${RUN_URL:+
${RUN_URL}}"

flags=(--parse-mode "" --no-preview --chat "$TG_TEST_CHAT")
[[ "$ok" -eq 1 ]] && flags+=(--silent)

if [[ "${DRY_RUN:-0}" == 1 ]]; then
  printf '%s\n' "$msg"
  echo "(flags: ${flags[*]})"
  exit 0
fi
"$TG" send "${flags[@]}" -q "$msg" >/dev/null
echo "notificación enviada"
