#!/usr/bin/env bash
# member-api 설정 시크릿(API 토큰)을 Vault KV v2 secret/member-api/config 에 쓴다(3편).
# VSO 가 이 값을 Kubernetes Secret member-api-config 의 api-token 키로 동기화한다.
# kv put 이므로 다시 실행하면 KV 버전이 하나 늘어난다.
set -euo pipefail
source "$(dirname "$0")/lib.sh"
require_context

if [ -f "$SM_DIR/.env" ]; then
  # shellcheck disable=SC1091
  source "$SM_DIR/.env"
fi
API_TOKEN="${API_TOKEN:-demo-api-token-v1}"

VAULT_TOKEN="$(root_token)" vault_exec kv put secret/member-api/config api-token="$API_TOKEN"
