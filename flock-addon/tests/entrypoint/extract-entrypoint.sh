#!/usr/bin/env bash
# Extract the embedded /bin/sh entrypoint from the rendered AddOnTemplate.
#
# helm-unittest can verify that the script's source contains certain text
# (we already do that in tests/addon-template_test.yaml), but it cannot run
# the script. The bats suites in this directory need a real, executable copy
# of the script so they can drive it with environment variables and assert
# observable behaviour. This helper renders the chart with `helm template`,
# pulls the literal block scalar out of the AddOnTemplate's container
# command, and writes it as a standalone shell script.
#
# Usage: extract-entrypoint.sh <variant: cpu|gpu> <output-path>
#
# We intentionally avoid any python yaml dependency: the addon-template only
# contains two literal block scalars (one per variant), and they are at a
# fixed indent in the rendered output, so awk + a state machine is enough.
set -euo pipefail

variant="${1:?variant (cpu|gpu) required}"
out="${2:?output path required}"

case "$variant" in
  cpu) wanted_index=0 ;;
  gpu) wanted_index=1 ;;
  *)
    echo "extract-entrypoint.sh: unknown variant: $variant" >&2
    exit 2
    ;;
esac

repo_root="$(cd "$(dirname "$0")"/../../.. && pwd)"
chart="$repo_root/flock-addon/charts/flock-addon"
helm="${HELM:-helm}"

rendered="$("$helm" template flock-addon "$chart")"

# State machine: when we see a line consisting solely of `- |` at the
# script's leading indent (22 spaces), start collecting. Stop on the first
# line that is non-empty and indented less than the script body's 24
# spaces — that is the start of the next field (`env:`).
printf '%s\n' "$rendered" | awk -v want="$wanted_index" '
  BEGIN { idx = -1; in_script = 0 }
  /^                      - \|$/ {
    idx++
    if (idx == want) { in_script = 1 }
    else if (idx > want) { exit 0 }
    next
  }
  in_script {
    if ($0 ~ /^                        /) {
      sub(/^                        /, "", $0)
      print
      next
    }
    if ($0 ~ /^[[:space:]]*$/) {
      print ""
      next
    }
    in_script = 0
  }
' > "$out"

if [ ! -s "$out" ]; then
  echo "extract-entrypoint.sh: extracted empty script from variant=$variant" >&2
  exit 1
fi
