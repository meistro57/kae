#!/usr/bin/env bash
set -euo pipefail

# ── KAE setup script ──────────────────────────────────────────────────────────
# Installs Go deps and starts Qdrant via Docker.

QDRANT_CONTAINER="kae-qdrant"
QDRANT_PORT=6333
QDRANT_IMAGE="qdrant/qdrant:latest"

echo "==> Checking dependencies..."

if ! command -v go &>/dev/null; then
  echo "ERROR: Go is not installed. Install it from https://go.dev/dl/" >&2
  exit 1
fi

if ! command -v docker &>/dev/null; then
  echo "ERROR: Docker is not installed. Install it from https://docs.docker.com/get-docker/" >&2
  exit 1
fi

# ── Go dependencies ───────────────────────────────────────────────────────────
echo "==> Downloading Go modules..."
go mod download

echo "==> Building kae..."
go build -o kae .

# ── .env file ─────────────────────────────────────────────────────────────────
if [[ ! -f .env ]]; then
  echo "==> Creating .env..."
  cat > .env <<EOF
OPENROUTER_API_KEY=
EOF
  echo "    NOTE: fill in your OPENROUTER_API_KEY in .env before running kae"
else
  echo "==> .env already exists, skipping"
fi

# ── Qdrant ────────────────────────────────────────────────────────────────────
echo "==> Checking Qdrant..."

if docker ps --format '{{.Names}}' | grep -q "^${QDRANT_CONTAINER}$"; then
  echo "    Qdrant is already running (${QDRANT_CONTAINER})"
else
  if docker ps -a --format '{{.Names}}' | grep -q "^${QDRANT_CONTAINER}$"; then
    echo "    Starting existing Qdrant container..."
    docker start "${QDRANT_CONTAINER}"
  else
    echo "    Pulling and starting Qdrant..."
    docker run -d \
      --name "${QDRANT_CONTAINER}" \
      --restart unless-stopped \
      -p "${QDRANT_PORT}:6333" \
      -v kae-qdrant-data:/qdrant/storage \
      "${QDRANT_IMAGE}"
  fi

  echo -n "    Waiting for Qdrant to be ready..."
  for i in $(seq 1 30); do
    if curl -sf "http://localhost:${QDRANT_PORT}/" &>/dev/null; then
      echo " ready."
      break
    fi
    echo -n "."
    sleep 1
    if [[ $i -eq 30 ]]; then
      echo ""
      echo "ERROR: Qdrant did not become ready in time. Check: docker logs ${QDRANT_CONTAINER}" >&2
      exit 1
    fi
  done
fi

# ── Done ──────────────────────────────────────────────────────────────────────
echo ""
echo "  Setup complete. Run kae with:"
echo "    ./kae"
echo "  or:"
echo "    go run ."
