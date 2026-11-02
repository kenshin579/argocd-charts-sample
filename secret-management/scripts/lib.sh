#!/usr/bin/env bash
# 모든 스크립트가 source 한다. kind-argo-cluster 외의 클러스터에는 명령을 보내지 않는다.
# (이 Mac 의 기본 kube context 가 다른 클러스터일 수 있으므로 모든 명령에 --context 를 붙인다)

KCTX="${KCTX:-kind-argo-cluster}"
SM_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INIT_FILE="${INIT_FILE:-$SM_DIR/.vault/init.json}"

require_context() {
  local contexts
  contexts="$(kubectl config get-contexts -o name)"
  if ! grep -qx "$KCTX" <<<"$contexts"; then
    echo "error: kube context '$KCTX' 가 없다. 'make cluster' 로 kind 클러스터를 먼저 만든다." >&2
    exit 1
  fi
}

k() {
  kubectl --context "$KCTX" "$@"
}

# vault-0 안의 vault CLI 를 실행한다. VAULT_TOKEN 은 호출하는 쪽 환경 변수에서 넘긴다.
vault_exec() {
  k -n vault exec -i vault-0 -- env VAULT_TOKEN="${VAULT_TOKEN:-}" vault "$@"
}

# vault status 는 sealed 면 exit 2 를 돌려주므로 실패로 보지 않는다.
vault_status_json() {
  k -n vault exec vault-0 -- vault status -format=json 2>/dev/null || true
}

root_token() {
  jq -r .root_token "$INIT_FILE"
}
