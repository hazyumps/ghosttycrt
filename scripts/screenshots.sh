#!/bin/sh
# Capture the current build using isolated demo config, fake SSH, and a real PTY.
# Python dependencies: python3 -m pip install -r scripts/requirements-screenshots.txt
# Set PYTHON to a virtual environment's interpreter when needed.
set -eu
root=$(cd "$(dirname "$0")/.." && pwd)
python=${PYTHON:-python3}
"$python" -c 'import PIL, pyte' || {
  echo 'Install scripts/requirements-screenshots.txt in your Python environment.' >&2
  exit 1
}
work=$(mktemp -d "${TMPDIR:-/tmp}/gcrt-screenshot-build.XXXXXX")
(cd "$root" && go build -o "$work/gcrt" ./cmd/gcrt)
exec "$python" -B "$root/scripts/screenshots.py" "$work/gcrt" "$root/docs/img"
