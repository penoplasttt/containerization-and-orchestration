#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-shop}"
INTERVAL="${INTERVAL:-3}"
LABELS=("app=shop-api" "app=shop-worker")

echo "Запускаю хаос: убиваю случайный под каждые ${INTERVAL}с в namespace ${NAMESPACE}"

while true; do
  LABEL="${LABELS[$((RANDOM % ${#LABELS[@]}))]}"

  POD=$(kubectl get pods -n "$NAMESPACE" -l "$LABEL" \
        --field-selector=status.phase=Running \
        -o jsonpath='{.items[*].metadata.name}' 2>/dev/null \
        | tr ' ' '\n' | awk 'BEGIN{srand()} {a[NR]=$0} END{print a[int(rand()*NR)+1]}')

  if [ -n "$POD" ]; then
    echo "[$(date +%H:%M:%S)] убиваю под $POD ($LABEL)"
    kubectl delete pod -n "$NAMESPACE" "$POD" --wait=false >/dev/null 2>&1
  else
    echo "[$(date +%H:%M:%S)] не нашёл под с лейблом $LABEL"
  fi

  sleep "$INTERVAL"
done