#!/usr/bin/env bash
# ============================================================================
# install.sh — installer for the tg CLI (https://github.com/xhelodev/tg)
#
# What it does (nothing else):
#   1. Downloads the source code from GitHub (or reuses a local clone,
#      updating it with a fast-forward pull).
#   2. Compiles it with the Go toolchain already on your machine.
#   3. Places the single static binary `tg` in your Go bin directory
#      (or TG_INSTALL_DIR / PREFIX if you set one).
#   4. If that directory is not in your PATH, appends one `export PATH=...`
#      line to your shell's rc file (~/.bashrc or ~/.zshrc), only once.
#
# What it does NOT do: no telemetry, no background processes, no writing
# outside $PREFIX, the repo clone and your shell rc file. Every network
# access goes to github.com over HTTPS. Read it before running it — that
# is the point of shipping a script instead of a curl|bash blob.
#
# Requirements: git and Go >= 1.22 in PATH.
# ============================================================================
set -euo pipefail

REPO_URL="https://github.com/xhelodev/tg.git"

usage() {
  cat <<'USAGE'
Usage: ./install.sh [options]

Installs the tg CLI (CGO_ENABLED=0) into PREFIX or TG_INSTALL_DIR
(default: $(go env GOPATH)/bin).

Options:
  -h, --help     Show this help
  --dry-run      Print actions without installing
  --dir DIR      Working clone directory (default: ./tg or current repo)
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

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --dry-run) DRY_RUN=1; shift ;;
    --dir) CLONE_DIR="${2:-}"; shift 2 ;;
    --ref) REF="${2:-}"; shift 2 ;;
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

# --- Check build dependencies (git + Go) ------------------------------------
need_cmd git
need_cmd go

GO_VER="$(go env GOVERSION 2>/dev/null || go version)"
echo "→ Go: $GO_VER"

# --- Decide where the binary goes -------------------------------------------
# Priority: TG_INSTALL_DIR > PREFIX > $(go env GOPATH)/bin (Go's default).
PREFIX="${TG_INSTALL_DIR:-${PREFIX:-}}"
if [[ -z "$PREFIX" ]]; then
  PREFIX="$(go env GOPATH)/bin"
fi
mkdir -p "$PREFIX" 2>/dev/null || true
echo "→ PREFIX=$PREFIX"

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
  # User-provided --dir, else ./tg next to where the script runs.
  if [[ -n "$CLONE_DIR" ]]; then
    echo "$CLONE_DIR"
    return
  fi
  echo "./tg"
}

REPO_DIR="$(resolve_repo_dir)"

# --- Get the source: clone fresh, or refresh an existing clone ---------------
ensure_clone() {
  if [[ -f "$REPO_DIR/cmd/tg/main.go" ]]; then
    echo "→ usando repo existente: $REPO_DIR"
    if [[ "$DRY_RUN" -eq 1 ]]; then
      echo "+ git -C $REPO_DIR fetch origin && git -C $REPO_DIR merge --ff-only <rama>"
    elif [[ -d "$REPO_DIR/.git" ]] && git -C "$REPO_DIR" fetch origin --prune 2>/tmp/tg-install-pull.err; then
      echo "→ actualizando clone desde origin"
      local branch
      branch="$(git -C "$REPO_DIR" rev-parse --abbrev-ref HEAD 2>/dev/null || echo HEAD)"
      if [[ "$branch" == "HEAD" ]]; then
        # HEAD detached: advance to origin/HEAD (fetch marks it not-for-merge,
        # so FETCH_HEAD is useless here).
        if git -C "$REPO_DIR" rev-parse -q --verify origin/HEAD >/dev/null 2>&1; then
          git -C "$REPO_DIR" merge --ff-only origin/HEAD 2>>/tmp/tg-install-pull.err \
            || echo "aviso: no se pudo avanzar el HEAD detached; se compila el estado actual" >&2
        else
          git -C "$REPO_DIR" checkout -q --detach origin/main 2>>/tmp/tg-install-pull.err \
            || echo "aviso: no se pudo avanzar el HEAD detached; se compila el estado actual" >&2
        fi
      elif ! git -C "$REPO_DIR" merge --ff-only "origin/$branch" 2>>/tmp/tg-install-pull.err; then
        # Diverged local branch: warn, but still build what is on disk.
        echo "aviso: no se pudo actualizar '$branch' (¿rama local divergida?); se compila el estado actual" >&2
        [[ -f /tmp/tg-install-pull.err ]] && cat /tmp/tg-install-pull.err >&2
      fi
    elif [[ "$DRY_RUN" -eq 0 ]]; then
      echo "aviso: no se pudo contactar origin (¿sin red?); se compila el estado actual" >&2
    fi
    return
  fi
  echo "→ clonando $REPO_URL → $REPO_DIR"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "+ git clone $REPO_URL $REPO_DIR"
    return
  fi
  if ! git clone "$REPO_URL" "$REPO_DIR" 2>/tmp/tg-install-clone.err; then
    cat >&2 <<EOF
error: no se pudo clonar $REPO_URL

Comprueba conexión a GitHub o clona manualmente:
  git clone $REPO_URL && cd tg && ./install.sh
EOF
    [[ -f /tmp/tg-install-clone.err ]] && cat /tmp/tg-install-clone.err >&2
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

# --- Build the static binary and drop it into $PREFIX ------------------------
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

echo "Prueba: tg login && tg me"
