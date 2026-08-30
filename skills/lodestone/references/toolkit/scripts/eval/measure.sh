#!/bin/bash
# Re-measure every labeled pair against the current skill.
#
#   TARGET=/path/to/labeled/repo skills/lodestone/references/toolkit/scripts/eval/measure.sh [work-dir]
#
# Plants each pair from testdata/pairs into a copy of the target repository
# (the bundled testdata/target by default),
# runs one report, and says where each pair landed. Pairs are split: settings
# were chosen against the tune pairs, so only the holdout rows are evidence that
# a setting generalises.
set -euo pipefail

TOOLKIT="$(cd "$(dirname "$0")/../.." && pwd)"
TARGET="${TARGET:-$TOOLKIT/testdata/target}"
WORK="${1:-${TMPDIR:-/tmp}/lodestone-measure}"
case "$WORK" in /|"${HOME:-}") echo "refusing to use $WORK as a work directory" >&2; exit 2;; esac
if [ -e "$WORK" ] && [ ! -e "$WORK/.lodestone-measure" ]; then
  echo "refusing to delete $WORK: it was not created by measure.sh" >&2
  exit 2
fi

echo "toolkit $TOOLKIT"
echo "target  $TARGET"
echo "work    $WORK"
echo

rm -rf "$WORK"
mkdir -p "$WORK"
touch "$WORK/.lodestone-measure"
cp -R "$TARGET" "$WORK/repo"
python3 "$TOOLKIT/scripts/eval/plant.py" --pairs "$TOOLKIT/testdata/pairs" --repo "$WORK/repo"
( cd "$WORK/repo" && go build ./... ) || { echo "planted repo does not build" >&2; exit 1; }
echo

if ! GOMAXPROCS=2 make -C "$TOOLKIT" analyze \
      REPOSITORY="$WORK/repo" PATTERNS='./...' FORMAT=json SOURCE_LINES=0 LIMIT=200 \
      OUTPUT="$WORK/report.json" >"$WORK/analyze.log" 2>&1; then
  echo "analyze failed; tail of $WORK/analyze.log:"
  tail -3 "$WORK/analyze.log" | sed 's/^/  /'
  exit 1
fi

python3 "$TOOLKIT/scripts/eval/score.py" --pairs "$TOOLKIT/testdata/pairs" --report "$WORK/report.json"
