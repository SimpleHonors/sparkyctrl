# Shared test helpers. Source this from each test_*.sh.
# Provides: sandbox(), cleanup, assert_eq, assert_contains, assert_file_mode, fail.
set -u

FAILS=0

fail() { echo "  FAIL: $*" >&2; FAILS=$((FAILS + 1)); }

assert_eq() { # expected actual msg
    if [ "$1" = "$2" ]; then echo "  ok: $3";
    else fail "$3 (expected '$1', got '$2')"; fi
}

assert_contains() { # haystack needle msg
    case "$1" in
        *"$2"*) echo "  ok: $3" ;;
        *) fail "$3 (missing '$2')" ;;
    esac
}

assert_file_mode() { # path expected-mode msg
    local m
    m=$(stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1" 2>/dev/null)
    assert_eq "$2" "$m" "$3"
}

count_occurrences() { grep -cF "$2" "$1" 2>/dev/null || echo 0; }

sandbox() { SBX=$(mktemp -d); echo "$SBX"; }
