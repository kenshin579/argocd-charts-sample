#!/usr/bin/env bash
# Kubernetes Secret member-api-config 의 api-token 과 Vault KV secret/member-api/config 의 값이 같은지 sha256 으로 비교한다.
set -euo pipefail
source "$(dirname "$0")/lib.sh"
require_context

secret_val="$(k -n member-api get secret member-api-config -o jsonpath='{.data.api-token}' | base64 -d)"
vault_val="$(VAULT_TOKEN="$(root_token)" vault_exec kv get -field=api-token secret/member-api/config)"
s="$(sha12 "$secret_val")"
v="$(sha12 "$vault_val")"
if [ "$s" = "$v" ]; then
  echo "match: secret=$s vault=$v"
else
  echo "MISMATCH: secret=$s vault=$v"
  exit 1
fi
