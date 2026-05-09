#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BAKFMT_BIN="${BAKFMT_BIN:-bin/bakfmt}"
if [[ ! -x "$BAKFMT_BIN" ]]; then
    echo "check_bakfmt: missing executable $BAKFMT_BIN" >&2
    echo "run: make build-bakfmt" >&2
    exit 1
fi

roots=("$@")
if [[ ${#roots[@]} -eq 0 ]]; then
    roots=(src/std examples tests)
fi

tmp="$(mktemp)"
err_tmp="$(mktemp)"
trap 'rm -f "$tmp" "$err_tmp"' EXIT

checked=0
skipped=0
failed=0

while IFS= read -r file; do
    if "$BAKFMT_BIN" "$file" >"$tmp" 2>"$err_tmp"; then
        checked=$((checked + 1))
        if ! cmp -s "$file" "$tmp"; then
            echo "$file"
            failed=1
        fi
        continue
    fi

    if [[ "$file" == src/std/* || "$file" == examples/* ]]; then
        cat "$err_tmp" >&2
        failed=1
        continue
    fi

    skipped=$((skipped + 1))
done < <(find "${roots[@]}" -type f -name '*.bak' | sort)

if [[ "$failed" -ne 0 ]]; then
    echo "Run: $BAKFMT_BIN -w ${roots[*]}" >&2
    echo "bakfmt checked $checked parseable file(s); skipped $skipped parser-error test fixture(s)." >&2
    exit 1
fi

echo "bakfmt checked $checked parseable file(s); skipped $skipped parser-error test fixture(s)."
