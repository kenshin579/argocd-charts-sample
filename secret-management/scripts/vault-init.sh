#!/usr/bin/env bash
# Vault 를 처음 한 번 초기화하고 unseal key 와 root token 을 .vault/init.json 에 저장한다.
# 이미 초기화돼 있으면 아무것도 쓰지 않는다(기존 unseal key 를 덮어쓰면 복구할 수 없다).
set -euo pipefail
source "$(dirname "$0")/lib.sh"
require_context

status="$(vault_status_json)"
if [ -z "$status" ]; then
  echo "error: vault-0 에 접속할 수 없다. 'make bootstrap' 후 Pod 가 Running 인지 확인한다." >&2
  exit 1
fi
if [ "$(jq -r .initialized <<<"$status")" = "true" ]; then
  echo "already initialized: $INIT_FILE 를 그대로 둔다"
  exit 0
fi

mkdir -p "$(dirname "$INIT_FILE")"
if [ -f "$INIT_FILE" ]; then
  # 클러스터를 새로 만든 경우다. 옛 파일은 지우지 않고 옆에 남긴다.
  mv "$INIT_FILE" "$INIT_FILE.$(date +%Y%m%d%H%M%S).bak"
fi

umask 077
k -n vault exec vault-0 -- vault operator init -key-shares=3 -key-threshold=2 -format=json > "$INIT_FILE.tmp"
mv "$INIT_FILE.tmp" "$INIT_FILE"
echo "initialized: unseal key 3개(threshold 2)와 root token 을 $INIT_FILE 에 저장했다"
