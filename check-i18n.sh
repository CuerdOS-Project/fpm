#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
keys="$(grep -RhoE 'tr\("[^"]+"\)' "$root" --include='*.go' | sed -E 's/tr\("([^"]+)"\)/\1/' | sort -u)"
status=0
for locale in "$root"/locales/*.kn; do
    while IFS= read -r key; do
        [[ -z "$key" ]] && continue
        if ! grep -q "^${key}=" "$locale"; then
            printf 'Missing key %s in %s\n' "$key" "$locale" >&2
            status=1
        fi
    done <<< "$keys"
done
if (( status == 0 )); then
    echo "Translation catalogs are complete."
fi
exit "$status"
