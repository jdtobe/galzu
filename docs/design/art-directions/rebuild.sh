#!/bin/sh
# Rebuild every study and its white-backed preview, then export the previews to
# PNG. Safe to run from anywhere; needs python3 and, for PNGs, Chrome.
set -e
cd "$(dirname "$0")"

for gen in generate6.py generate7.py; do
    echo "== $gen"
    python3 "$gen" >/dev/null
done

echo "== preview.py"
python3 preview.py

echo "open preview/ to look at anything; the files above it stay transparent"
