#!/usr/bin/env bash
set -e

PORT=${1:-5150}
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
URL="http://localhost:$PORT/kae-3d-viewer.html"

echo "Serving KAE 3D Viewer at $URL"
echo "Press Ctrl+C to stop."

# Try to open the browser (WSL-friendly)
if command -v wslview &>/dev/null; then
    wslview "$URL" &
elif command -v xdg-open &>/dev/null; then
    xdg-open "$URL" &
fi

cd "$DIR"
python3 -m http.server "$PORT"
