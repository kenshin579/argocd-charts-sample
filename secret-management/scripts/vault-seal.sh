#!/usr/bin/env bash
# 데모용: vault-0 을 seal 한다(재시작한 것과 같은 상태). 다시 쓰려면 make vault-unseal
set -euo pipefail
source "$(dirname "$0")/lib.sh"
require_context

VAULT_TOKEN="$(root_token)" vault_exec operator seal
