#!/usr/bin/env bash
# ============================================================================
# install.sh — installer for the tg CLI (https://github.com/xhelodev/tg)
#
# What it does (nothing else):
#   1. Downloads the source code from GitHub into one fixed directory
#      (default ~/.local/share/tg/src), or reuses that clone / the repo you
#      run it from, updating it with a fast-forward pull.
#   2. Compiles it with the Go toolchain already on your machine.
#   3. Places the single static binary `tg` in your Go bin directory
#      (or TG_INSTALL_DIR / PREFIX if you set one).
#   4. If that directory is not in your PATH, appends one `export PATH=...`
#      line to your shell's rc file (~/.bashrc or ~/.zshrc), only once.
#
# What it does NOT do: no telemetry, no background processes, no writing
# outside $PREFIX, the repo clone, your shell rc file and (with --skill)
# the agents' skill directories. Every network
# access goes to github.com over HTTPS. Read it before running it — that
# is the point of shipping a script instead of a curl|bash blob.
#
# Requirements: git and Go >= 1.22 in PATH (only git for --skill-only).
# ============================================================================
set -euo pipefail

# Where the user invoked the script: Cursor project-rules are looked up here,
# even though the script later cd's into the repo clone to build.
INVOCATION_DIR="$PWD"

REPO_URL="https://github.com/xhelodev/tg.git"
# Fixed clone location, so curl|bash doesn't leave a ./tg wherever it runs.
DEFAULT_SRC_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/tg/src"

usage() {
  cat <<'USAGE'
Usage: ./install.sh [options]

Installs the tg CLI (CGO_ENABLED=0) into PREFIX or TG_INSTALL_DIR
(default: $(go env GOPATH)/bin).

Options:
  -h, --help     Show this help
  --dry-run      Print actions without installing
  --skill        Also install the `tg` agent skill for detected agents
  --skill-only   Install only the agent skill (no build, no PATH changes)
  --dir DIR      Clone directory (default: current repo, else
                 ${XDG_DATA_HOME:-~/.local/share}/tg/src — keep in sync
                 with DEFAULT_SRC_DIR)
  --ref REF      git ref to checkout (default: HEAD / default branch)

Environment:
  TG_INSTALL_DIR / PREFIX   Install directory for the binary
  TG_REPO_DIR               If set and is this repo, use it instead of cloning
  TG_NO_PATH=1              Do not touch shell rc files (skip PATH setup)

Examples:
  git clone https://github.com/xhelodev/tg.git && cd tg && ./install.sh
  curl -fsSL https://raw.githubusercontent.com/xhelodev/tg/main/install.sh | bash
  TG_INSTALL_DIR=$HOME/.local/bin ./install.sh
  ./install.sh --dry-run
USAGE
}

# --- Parse command-line options -------------------------------------------
DRY_RUN=0
CLONE_DIR=""
REF=""
WANT_SKILL=0   # --skill:     install CLI + agent skill
SKILL_ONLY=0   # --skill-only: skip build/PATH, install just the skill

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --dry-run) DRY_RUN=1; shift ;;
    --skill) WANT_SKILL=1; shift ;;
    --skill-only) WANT_SKILL=1; SKILL_ONLY=1; shift ;;
    --dir|--ref)
      if [[ $# -lt 2 || -z "$2" || "$2" == -* ]]; then
        echo "error: $1 requiere un valor" >&2; usage >&2; exit 2
      fi
      if [[ "$1" == --dir ]]; then CLONE_DIR="$2"; else REF="$2"; fi
      shift 2 ;;
    *) echo "error: unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

# --- Small helpers ----------------------------------------------------------
# run(): executes a command, or just prints it in --dry-run mode.
run() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "+ $*"
  else
    "$@"
  fi
}

die() { echo "error: $*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "se necesita '$1' en PATH"
}

# Private scratch file for git's stderr (shown only on failure), removed on exit.
ERR_LOG="$(mktemp "${TMPDIR:-/tmp}/tg-install.XXXXXX")"
trap 'rm -f "$ERR_LOG"' EXIT

# --- Check build dependencies (git + Go) ------------------------------------
# --skill-only doesn't build anything: git is enough there.
need_cmd git
if [[ "$SKILL_ONLY" -eq 0 ]]; then
  need_cmd go
  GO_VER="$(go env GOVERSION 2>/dev/null || go version)"
  echo "→ Go: $GO_VER"
fi

# --- Decide where the binary goes -------------------------------------------
# Priority: TG_INSTALL_DIR > PREFIX > $(go env GOPATH)/bin (Go's default).
# Skipped for --skill-only, which must work without Go.
if [[ "$SKILL_ONLY" -eq 0 ]]; then
  PREFIX="${TG_INSTALL_DIR:-${PREFIX:-}}"
  if [[ -z "$PREFIX" ]]; then
    PREFIX="$(go env GOPATH)/bin"
  fi
  mkdir -p "$PREFIX" 2>/dev/null || true
  echo "→ PREFIX=$PREFIX"
fi

# --- Locate the source code --------------------------------------------------
# Reuse an existing checkout when we can, so a second run never re-downloads.
resolve_repo_dir() {
  # Explicit override: TG_REPO_DIR pointing at a real tg checkout.
  if [[ -n "${TG_REPO_DIR:-}" && -f "${TG_REPO_DIR}/cmd/tg/main.go" ]]; then
    echo "$TG_REPO_DIR"
    return
  fi
  # Already inside the tg repo? Use the current directory.
  if [[ -f "./cmd/tg/main.go" && -f "./go.mod" ]]; then
    pwd
    return
  fi
  # User-provided --dir, else the fixed per-user location.
  if [[ -n "$CLONE_DIR" ]]; then
    echo "$CLONE_DIR"
    return
  fi
  echo "$DEFAULT_SRC_DIR"
}

REPO_DIR="$(resolve_repo_dir)"

# --- Get the source: clone fresh, or refresh an existing clone ---------------
ensure_clone() {
  if [[ -f "$REPO_DIR/cmd/tg/main.go" ]]; then
    echo "→ usando repo existente: $REPO_DIR"
    if [[ "$DRY_RUN" -eq 1 ]]; then
      echo "+ git -C $REPO_DIR fetch origin && git -C $REPO_DIR merge --ff-only <rama>"
    elif [[ -d "$REPO_DIR/.git" ]] && git -C "$REPO_DIR" fetch origin --prune 2>"$ERR_LOG"; then
      echo "→ actualizando clone desde origin"
      local branch
      branch="$(git -C "$REPO_DIR" rev-parse --abbrev-ref HEAD 2>/dev/null || echo HEAD)"
      if [[ "$branch" == "HEAD" ]]; then
        # HEAD detached: advance to origin/HEAD (fetch marks it not-for-merge,
        # so FETCH_HEAD is useless here).
        if git -C "$REPO_DIR" rev-parse -q --verify origin/HEAD >/dev/null 2>&1; then
          git -C "$REPO_DIR" merge --ff-only origin/HEAD 2>>"$ERR_LOG" \
            || echo "aviso: no se pudo avanzar el HEAD detached; se compila el estado actual" >&2
        else
          git -C "$REPO_DIR" checkout -q --detach origin/main 2>>"$ERR_LOG" \
            || echo "aviso: no se pudo avanzar el HEAD detached; se compila el estado actual" >&2
        fi
      elif ! git -C "$REPO_DIR" merge --ff-only "origin/$branch" 2>>"$ERR_LOG"; then
        # Diverged local branch: warn, but still build what is on disk.
        echo "aviso: no se pudo actualizar '$branch' (¿rama local divergida?); se compila el estado actual" >&2
        cat "$ERR_LOG" >&2
      fi
    elif [[ "$DRY_RUN" -eq 0 && ! -d "$REPO_DIR/.git" ]]; then
      echo "aviso: $REPO_DIR no es un clone git; se compila tal cual (bórralo para re-clonar)" >&2
    elif [[ "$DRY_RUN" -eq 0 ]]; then
      echo "aviso: no se pudo contactar origin (¿sin red?); se compila el estado actual" >&2
      cat "$ERR_LOG" >&2
    fi
    return
  fi
  echo "→ clonando $REPO_URL → $REPO_DIR"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "+ git clone $REPO_URL $REPO_DIR"
    return
  fi
  mkdir -p "$(dirname "$REPO_DIR")"
  if ! git clone "$REPO_URL" "$REPO_DIR" 2>"$ERR_LOG"; then
    cat >&2 <<EOF
error: no se pudo clonar $REPO_URL

Comprueba conexión a GitHub o clona manualmente:
  git clone $REPO_URL && cd tg && ./install.sh
EOF
    cat "$ERR_LOG" >&2
    exit 1
  fi
}

ensure_clone

# --- Sanity-check the source tree before building ----------------------------
if [[ "$DRY_RUN" -eq 0 ]]; then
  cd "$REPO_DIR"
  # Optional --ref: build a specific tag/branch/commit instead of HEAD.
  if [[ -n "$REF" ]]; then
    git fetch --tags origin 2>/dev/null || true
    git checkout "$REF"
  fi
  if [[ ! -f go.mod || ! -f cmd/tg/main.go ]]; then
    die "no parece el repo tg (falta go.mod o cmd/tg/main.go)"
  fi
else
  echo "+ cd $REPO_DIR"
fi

# --- Install the agent skill (skills/tg/SKILL.md) ----------------------------
# skills/tg is the *usage* skill (send alerts with tg from any project); the
# repo-workflow skill (skills/tg-dev) is project-level only, see README.
# Copies the repo's SKILL.md into the skills/rules directory of every coding
# agent detected on this machine. Detection = the agent's config dir exists:
#   Claude Code  -> ~/.claude/skills/tg/
#   OpenCode     -> ~/.config/opencode/skills/tg/
#   Hermes       -> ~/.hermes/skills/tg/
#   Codex/ChatGPT-> ~/.codex/skills/tg/
# If none is found, falls back to ~/.claude/skills/tg. Cursor has no global
# rules dir, so it is only installed project-level when ./.cursor/rules exists.
# Copying (not symlinking) keeps each agent self-contained; re-run to update
# (the clone is refreshed first, so a re-run always installs the latest).
install_skill() {
  local src="$REPO_DIR/skills/tg/SKILL.md"
  if [[ ! -f "$src" ]]; then
    echo "aviso: no se encontró $src; actualiza el clone (git pull)" >&2
    return 1
  fi
  local -a dests=()
  [[ -d "$HOME/.claude" ]]          && dests+=("$HOME/.claude/skills/tg")
  [[ -d "$HOME/.config/opencode" ]] && dests+=("$HOME/.config/opencode/skills/tg")
  [[ -d "$HOME/.hermes" ]]          && dests+=("$HOME/.hermes/skills/tg")
  [[ -d "$HOME/.codex" ]]           && dests+=("$HOME/.codex/skills/tg")
  if [[ ${#dests[@]} -eq 0 ]]; then
    # No agent detected: default to Claude Code's location.
    dests+=("$HOME/.claude/skills/tg")
  fi
  local dest
  for dest in "${dests[@]}"; do
    if [[ "$DRY_RUN" -eq 1 ]]; then
      echo "+ mkdir -p $dest && cp $src $dest/SKILL.md"
    else
      mkdir -p "$dest"
      cp "$src" "$dest/SKILL.md"
      echo "→ skill instalada en $dest/SKILL.md"
    fi
  done
  # Cursor: rules are project-level (.mdc with its own frontmatter), resolved
  # against the directory where install.sh was invoked.
  if [[ -d "$INVOCATION_DIR/.cursor/rules" ]]; then
    local mdc="$INVOCATION_DIR/.cursor/rules/tg.mdc"
    if ! command -v awk >/dev/null 2>&1; then
      # Without awk we can't strip the skill's frontmatter; a .mdc with two
      # frontmatter blocks would be malformed, so skip it.
      echo "aviso: sin awk no se instala la regla de Cursor ($mdc)" >&2
    elif [[ "$DRY_RUN" -eq 1 ]]; then
      echo "+ (frontmatter + skill) > $mdc"
    else
      {
        # Cursor reads its own frontmatter: drop the skill's, keep the body
        # (tolerates CRLF).
        printf -- '---\ndescription: Send Telegram messages, alerts and files with the tg CLI; find a chat_id\nglobs:\nalwaysApply: false\n---\n'
        awk '{ l = $0; sub(/\r$/, "", l) } NR==1 && l == "---" {fm=1; next} fm && l == "---" {fm=0; next} !fm' "$src"
      } > "$mdc"
      echo "→ skill instalada como regla de Cursor en $mdc"
    fi
  fi
}

# --- Build the static binary and drop it into $PREFIX ------------------------
if [[ "$SKILL_ONLY" -eq 1 ]]; then
  echo "→ --skill-only: saltando build y PATH; instalando solo la skill"
  install_skill || exit 1
  exit 0
fi

# Stamp version/commit into the binary so `tg version` can report them.
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
echo "→ go build (CGO_ENABLED=0) PREFIX=$PREFIX"
if [[ "$DRY_RUN" -eq 1 ]]; then
  echo "+ CGO_ENABLED=0 go build -ldflags \"-s -w -X main.version=$VERSION -X main.commit=$COMMIT\" -o \"$PREFIX/tg\" ./cmd/tg"
  echo "(dry-run OK)"
  exit 0
fi

CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" -o "$PREFIX/tg" ./cmd/tg

BIN="$PREFIX/tg"
if [[ ! -x "$BIN" ]]; then
  die "binario no encontrado en $PREFIX/tg tras go build"
fi

echo "✓ instalado: $BIN"
"$BIN" version || true

# --- Make sure `tg` is callable ----------------------------------------------
# If $PREFIX is missing from PATH, append one export line to the login
# shell's rc file ($SHELL decides: zsh -> ~/.zshrc, bash -> ~/.bashrc).
# Idempotent: never duplicates the line; TG_NO_PATH=1 skips this entirely.
ensure_path() {
  if [[ ":$PATH:" == *":$PREFIX:"* ]]; then
    echo "→ $PREFIX ya está en PATH"
    return
  fi
  if [[ "${TG_NO_PATH:-0}" == "1" ]]; then
    echo "→ TG_NO_PATH=1: no se modifica ningún rc"
    return
  fi
  local shell_name rc
  shell_name="$(basename "${SHELL:-bash}")"
  case "$shell_name" in
    zsh)
      rc="$HOME/.zshrc"
      ;;
    bash)
      # bash login shells on macOS read .bash_profile; prefer it when
      # .bashrc is absent but .bash_profile exists.
      if [[ ! -f "$HOME/.bashrc" && -f "$HOME/.bash_profile" ]]; then
        rc="$HOME/.bash_profile"
      else
        rc="$HOME/.bashrc"
      fi
      ;;
    *)
      # Unknown shell: don't guess, print the line for the user to add.
      echo "aviso: shell '$shell_name' no soportada; añade a tu rc:"
      echo "  export PATH=\"$PREFIX:\$PATH\""
      return
      ;;
  esac
  if grep -qsF "$PREFIX" "$rc"; then
    echo "→ $rc ya referencia $PREFIX (abre una shell nueva)"
    return
  fi
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "+ echo 'export PATH=\"$PREFIX:\$PATH\"' >> $rc"
    return
  fi
  {
    echo ""
    echo "# added by tg install.sh"
    echo "export PATH=\"$PREFIX:\$PATH\""
  } >> "$rc"
  echo "→ añadido export PATH a $rc"
  echo "  (recarga con: source $rc)"
}

ensure_path

if [[ "$WANT_SKILL" -eq 1 ]]; then
  # The binary is already installed: a skill problem is a warning, not a failure.
  install_skill || echo "aviso: la skill no se instaló; el binario sí" >&2
fi

echo "Prueba: tg login && tg me"
