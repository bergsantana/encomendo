#!/usr/bin/env bash
# Coverage gate from docs/architecture.md section 9:
#   >= 90% on internal/domain + internal/app, >= 80% overall.
#
# Usage: scripts/coverage-gate.sh unit.out [integration.out ...]
#
# Profiles must come from `go test -coverpkg=./internal/... -coverprofile=...`
# so every profile describes the same blocks. A block counts as covered when
# any profile covered it. cmd/ is left out: composition roots are exercised by
# the e2e suite, not by unit tests.
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 profile.out [more.out ...]" >&2
  exit 2
fi

MODULE="github.com/bergsantana/encomendo/backend"
CORE_MIN=90
OVERALL_MIN=80

awk -v module="$MODULE" -v core_min="$CORE_MIN" -v overall_min="$OVERALL_MIN" '
  FNR == 1 && /^mode:/ { next }
  {
    # file:start,end numStmts count
    block = $1; stmts = $2; count = $3
    if (!(block in seen)) { seen[block] = stmts; hit[block] = 0 }
    if (count > 0) { hit[block] = 1 }
  }
  END {
    for (block in seen) {
      file = block; sub(/:.*/, "", file)
      if (index(file, module "/internal/") != 1) continue
      total += seen[block]; covered += hit[block] * seen[block]
      if (index(file, module "/internal/domain/") == 1 || index(file, module "/internal/app/") == 1) {
        core_total += seen[block]; core_covered += hit[block] * seen[block]
      }
    }
    failed = 0
    if (total == 0) { print "overall: no statements"; exit 0 }
    overall = 100 * covered / total
    printf "overall (internal/...):        %.1f%% (minimum %d%%)\n", overall, overall_min
    if (overall < overall_min) failed = 1
    if (core_total > 0) {
      core = 100 * core_covered / core_total
      printf "domain + app (internal/{domain,app}): %.1f%% (minimum %d%%)\n", core, core_min
      if (core < core_min) failed = 1
    } else {
      print "domain + app: no statements yet"
    }
    if (failed) { print "coverage gate FAILED"; exit 1 }
    print "coverage gate passed"
  }
' "$@"
