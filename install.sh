#!/usr/bin/env bash
# install.sh — clona (o reutiliza) github.com/xhelodev/tg e instala el binario tg.
# Repo público: requiere git + Go ≥ 1.22. Si ya hay un clone local, no vuelve a clonar.
set -euo pipefail

REPO_URL="https://github.com/xhelodev/tg.git"
MODULE="github.com/xhelodev/tg"

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

Examples:
  git clone https://github.com/xhelodev/tg.git && cd tg && ./install.sh
  curl -fsSL https://raw.githubusercontent.com/xhelodev/tg/main/install.sh | bash
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

ensure_clone() {
  if [[ -f "$REPO_DIR/cmd/tg/main.go" ]]; then
    echo "→ usando repo existente: $REPO_DIR"
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

if [[ "$DRY_RUN" -eq 0 ]]; then
  cd "$REPO_DIR"
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
echo "Prueba: tg login && tg me"
