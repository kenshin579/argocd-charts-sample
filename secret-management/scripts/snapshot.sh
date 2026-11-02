#!/usr/bin/env bash
# Raft snapshot 저장과 복구. Transit KEK 는 Vault 밖으로 꺼낼 수 없으므로 이 snapshot 이 곧 KEK 백업이다.
#   ./scripts/snapshot.sh save               → .vault/snapshots/vault-<시각>.snap
#   ./scripts/snapshot.sh restore <파일>
# snapshot 은 이 클러스터의 unseal key 로만 쓸 수 있다. .vault/init.json 과 함께 보관한다.
set -euo pipefail
source "$(dirname "$0")/lib.sh"
require_context

VAULT_TOKEN="$(root_token)"
export VAULT_TOKEN
dir="$SM_DIR/.vault/snapshots"

case "${1:-}" in
  save)
    mkdir -p "$dir"
    file="$dir/vault-$(date +%Y%m%d%H%M%S).snap"
    vault_exec operator raft snapshot save /tmp/vault.snap
    k -n vault cp vault-0:/tmp/vault.snap "$file" > /dev/null
    k -n vault exec vault-0 -- rm -f /tmp/vault.snap
    echo "saved: $file ($(wc -c < "$file" | tr -d ' ') bytes)"
    ;;
  restore)
    file="${2:?usage: snapshot.sh restore <file>}"
    k -n vault cp "$file" vault-0:/tmp/restore.snap > /dev/null
    vault_exec operator raft snapshot restore /tmp/restore.snap
    k -n vault exec vault-0 -- rm -f /tmp/restore.snap
    echo "restored: $file (Vault 로그에서 'post-unseal setup complete', 'vault is unsealed' 를 확인한 뒤 사용한다)"
    ;;
  *)
    echo "usage: snapshot.sh save | restore <file>" >&2
    exit 1
    ;;
esac
