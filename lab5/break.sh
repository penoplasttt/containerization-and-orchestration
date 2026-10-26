#!/usr/bin/env bash

set -euo pipefail

CHART_DIR="$(cd "$(dirname "$0")" && pwd)/shop"
VALUES="$CHART_DIR/values.yaml"
BACKUP="$CHART_DIR/values.yaml.bak"
LOG="/tmp/breakage.log"

VARIANTS=(break-port break-labels break-dns break-client-port)

find_python() {
  for p in python3 python /opt/anaconda3/bin/python3 /usr/bin/python3; do
    if command -v "$p" >/dev/null 2>&1 && "$p" -c "import yaml" >/dev/null 2>&1; then
      echo "$p"
      return 0
    fi
  done
  return 1
}

PYTHON="$(find_python)"
if [ -z "$PYTHON" ]; then
  echo "ОШИБКА: не найден python с модулем yaml. Установи: pip install pyyaml" >&2
  exit 1
fi

restore() {
  if [ -f "$BACKUP" ]; then
    cp "$BACKUP" "$VALUES"
    echo "values.yaml восстановлен из бэкапа"
    helm upgrade shop "$CHART_DIR" -n shop >/dev/null 2>&1
    echo "чарт переприменён (поломка снята)"
  else
    echo "бэкап не найден: $BACKUP"
  fi
}

if [ "${1:-}" = "restore" ]; then
  restore
  exit 0
elif [ -n "${1:-}" ]; then
  VARIANT="$1"
else
  VARIANT="${VARIANTS[$((RANDOM % ${#VARIANTS[@]}))]}"
fi

if [ ! -f "$BACKUP" ]; then
  cp "$VALUES" "$BACKUP"
  echo "создан бэкап: $BACKUP"
fi

echo "применяю поломку: $VARIANT"

"$PYTHON" - "$VALUES" "$VARIANT" <<'PY'
import sys
import yaml

path, variant = sys.argv[1], sys.argv[2]

with open(path) as f:
    data = yaml.safe_load(f)

np = data["networkPolicy"]
ingress = np.get("ingress", [])
egress = np.get("egress", [])

def find(lst, name):
    return next((x for x in lst if x.get("name") == name), None)

if variant == "break-port":
    # eg-allow-api-to-postgres: порт 5432 -> 5433
    r = find(egress, "eg-allow-api-to-postgres")
    r["ports"][0]["port"] = 5433

elif variant == "break-labels":
    # eg-allow-api-to-postgres: лейбл shop-api -> shop-worker (api не попадёт в правило)
    r = find(egress, "eg-allow-api-to-postgres")
    r["podSelector"]["matchLabels"]["app"] = "shop-worker"

elif variant == "break-dns":
    # удаляем eg-allow-dns
    np["egress"] = [r for r in egress if r.get("name") != "eg-allow-dns"]

elif variant == "break-client-port":
    # ing-allow-client-to-api: порт 8080 -> 8081
    r = find(ingress, "ing-allow-client-to-api")
    r["ports"][0]["port"] = 8081

else:
    sys.exit(f"неизвестный вариант: {variant}")

with open(path, "w") as f:
    yaml.safe_dump(data, f, sort_keys=False)

print(f"values.yaml обновлён: {variant}")
PY

helm upgrade shop "$CHART_DIR" -n shop 2>&1 | tail -3

echo "$(date +%H:%M:%S) $VARIANT" >> "$LOG"
echo ""
echo "Поломка внесена"