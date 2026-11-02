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

for key in $(jq -r '.unseal_keys_b64[0:2][]' "$INIT_FILE"); do
  k -n vault exec vault-0 -- vault operator unseal "$key" > /dev/null
done
echo "unsealed: sealed=$(vault_status_json | jq -r .sealed)"
