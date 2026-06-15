#!/usr/bin/env bash
# Run all unraid deploy tests. Exit non-zero if any test reports a failure.
set -u
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOTAL_FAILS=0
for t in "$HERE"/test_*.sh; do
    echo "=== $(basename "$t") ==="
    FAILS=0
    # shellcheck source=/dev/null
    . "$t"
    TOTAL_FAILS=$((TOTAL_FAILS + FAILS))
done
echo "=================="
if [ "$TOTAL_FAILS" -gt 0 ]; then
    echo "FAILED: $TOTAL_FAILS assertion(s)"; exit 1
fi
echo "ALL TESTS PASSED"
