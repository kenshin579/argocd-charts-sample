#!/usr/bin/env bash
# .vault/init.json 의 unseal key 2개로 vault-0 을 unseal 한다. 이미 unseal 돼 있으면 건너뛴다.
set -euo pipefail
source "$(dirname "$0")/lib.sh"
require_context

status="$(vault_status_json)"
if [ "$(jq -r .sealed <<<"$status")" = "false" ]; then
  echo "already unsealed"
  exit 0
fi

if [ ! -f "$INIT_FILE" ]; then
  echo "error: $INIT_FILE 가 없다. 이 Vault 를 초기화할 때 저장한 unseal key 가 필요하다." >&2
  exit 1
fi

keys="$(jq -r '.unseal_keys_b64[0:2][]' "$INIT_FILE")"
for key in $keys; do
  k -n vault exec vault-0 -- vault operator unseal "$key" > /dev/null
done

sealed="$(vault_status_json | jq -r .sealed)"
if [ "$sealed" != "false" ]; then
  echo "error: unseal 후에도 sealed=$sealed 다. $INIT_FILE 가 이 Vault 의 것인지 확인한다." >&2
  exit 1
fi
echo "unsealed: sealed=false"
