#!/usr/bin/env bash
# install.sh — clona (o reutiliza) github.com/xhelodev/tg e instala el binario tg.
# Repo privado: hace falta git/gh autenticado + Go ≥ 1.22.
# Si ya hay un clone local, la instalación es offline (sin tocar la red).
set -euo pipefail

REPO_SSH="git@github.com:xhelodev/tg.git"
REPO_HTTPS="https://github.com/xhelodev/tg.git"
MODULE="github.com/xhelodev/tg"
DEFAULT_GOPRIVATE="github.com/xhelodev/*"

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
  GOPRIVATE                 Defaulted to github.com/xhelodev/* if unset
  TG_REPO_DIR               If set and is this repo, use it instead of cloning

Examples:
  git clone git@github.com:xhelodev/tg.git && cd tg && ./install.sh
  TG_INSTALL_DIR=$HOME/.local/bin ./install.sh
  ./install.sh --dry-run
USAGE
}

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

need_cmd git
need_cmd go

GO_VER="$(go env GOVERSION 2>/dev/null || go version)"
echo "→ Go: $GO_VER"

export GOPRIVATE="${GOPRIVATE:-$DEFAULT_GOPRIVATE}"
echo "→ GOPRIVATE=$GOPRIVATE"

PREFIX="${TG_INSTALL_DIR:-${PREFIX:-}}"
if [[ -z "$PREFIX" ]]; then
  PREFIX="$(go env GOPATH)/bin"
fi
mkdir -p "$PREFIX" 2>/dev/null || true
echo "→ PREFIX=$PREFIX"

# Detect if we are already inside the tg repo
resolve_repo_dir() {
  if [[ -n "${TG_REPO_DIR:-}" && -f "${TG_REPO_DIR}/cmd/tg/main.go" ]]; then
    echo "$TG_REPO_DIR"
    return
  fi
  if [[ -f "./cmd/tg/main.go" && -f "./go.mod" ]]; then
    pwd
    return
  fi
  if [[ -n "$CLONE_DIR" ]]; then
    echo "$CLONE_DIR"
    return
  fi
  echo "./tg"
}

REPO_DIR="$(resolve_repo_dir)"

# Clona solo si no hay árbol local. Preferimos SSH (auth típica en VPS), luego HTTPS.
ensure_clone() {
  if [[ -f "$REPO_DIR/cmd/tg/main.go" ]]; then
    echo "→ usando repo existente: $REPO_DIR"
    return
  fi
  echo "→ clonando $REPO_SSH → $REPO_DIR (SSH primero; HTTPS de respaldo)"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "+ git clone $REPO_SSH $REPO_DIR"
    echo "+ # si falla: git clone $REPO_HTTPS $REPO_DIR"
    return
  fi
  if git clone "$REPO_SSH" "$REPO_DIR" 2>/tmp/tg-install-clone.err; then
    return
  fi
  echo "aviso: clone SSH falló; intentando HTTPS…" >&2
  if git clone "$REPO_HTTPS" "$REPO_DIR" 2>>/tmp/tg-install-clone.err; then
    return
  fi
  cat >&2 <<'AUTH'
error: no se pudo clonar el repo privado github.com/xhelodev/tg.

Necesitas autenticación GitHub + acceso al repo, por ejemplo:
  # SSH (recomendado en VPS)
  ssh -T git@github.com
  # o
  gh auth login
  # o
  git config --global url."https://x-access-token:TOKEN@github.com/".insteadOf "https://github.com/"

También:
  export GOPRIVATE=github.com/xhelodev/*
AUTH
  [[ -f /tmp/tg-install-clone.err ]] && cat /tmp/tg-install-clone.err >&2
  exit 1
}

ensure_clone

if [[ "$DRY_RUN" -eq 0 ]]; then
  cd "$REPO_DIR"
  if [[ -n "$REF" ]]; then
    git fetch --tags origin 2>/dev/null || true
    git checkout "$REF"
  fi
  if [[ ! -f go.mod || ! -f cmd/tg/main.go ]]; then
    die "no parece el repo tg (falta go.mod o cmd/tg)"
  fi
else
  echo "+ cd $REPO_DIR"
fi

# Instalación desde árbol local: go build offline, sin tocar la red.
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
LDFLAGS="-s -w -X main.version=$VERSION -X main.commit=$COMMIT"
echo "→ go build (CGO_ENABLED=0) PREFIX=$PREFIX"
if [[ "$DRY_RUN" -eq 1 ]]; then
  echo "+ CGO_ENABLED=0 go build -ldflags \"$LDFLAGS\" -o \"$PREFIX/tg\" ./cmd/tg"
  echo "(dry-run OK)"
  exit 0
fi

CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o "$PREFIX/tg" ./cmd/tg

BIN="$PREFIX/tg"
if [[ ! -x "$BIN" ]]; then
  die "binario no encontrado en $PREFIX/tg tras go build"
fi

echo "✓ instalado: $BIN"
"$BIN" version || true
echo "Prueba: tg login && tg me"
