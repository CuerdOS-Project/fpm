#!/bin/bash
# Verifica que todas las claves t("...") usadas en src/*.c existan en cada locales/*.kn
# Uso: ./check-i18n.sh

set -e
cd "$(dirname "$0")"

used_keys=$(grep -ohE 't\("[a-zA-Z0-9_]+"\)' src/*.c | sed -E 's/t\("([^"]+)"\)/\1/' | sort -u)

status=0
for f in locales/*.kn; do
    have_keys=$(grep -E '^[a-zA-Z0-9_]+=' "$f" | cut -d= -f1 | sort -u)
    missing=$(comm -23 <(echo "$used_keys") <(echo "$have_keys"))
    if [ -n "$missing" ]; then
        echo "FALTAN claves en $f:"
        echo "$missing" | sed 's/^/  - /'
        status=1
    fi
done

if [ "$status" -eq 0 ]; then
    echo "OK: todos los catálogos tienen las claves usadas en el código."
fi

exit $status
