#!/usr/bin/env bash
# Vault 관리자(인프라 담당) 역할의 초기 설정. root token 으로 한 번 실행하고, 다시 실행해도 결과가 같다.
#   - secret engine: kv-v2(secret/), transit(transit/)
#   - KEK: transit/keys/member-api-kek
#   - policy: member-api(앱), member-api-migration(DEK 적재·교체), vso-member-api(설정 시크릿 동기화, 3편)
#   - auth: kubernetes, role member-api / member-api-migration / vso-member-api
#   - audit: file(stdout)
set -euo pipefail
source "$(dirname "$0")/lib.sh"
require_context

VAULT_TOKEN="$(root_token)"
export VAULT_TOKEN

secrets="$(vault_exec secrets list -format=json)"
if ! jq -e 'has("secret/")' <<<"$secrets" > /dev/null; then
  vault_exec secrets enable -path=secret kv-v2
fi
if ! jq -e 'has("transit/")' <<<"$secrets" > /dev/null; then
  vault_exec secrets enable transit
fi

vault_exec write -f transit/keys/member-api-kek > /dev/null
echo "Transit KEK ready: member-api-kek"

for policy in member-api member-api-migration vso-member-api; do
  vault_exec policy write "$policy" - < "$SM_DIR/vault/policies/$policy.hcl"
done

auths="$(vault_exec auth list -format=json)"
if ! jq -e 'has("kubernetes/")' <<<"$auths" > /dev/null; then
  vault_exec auth enable kubernetes
fi
# Vault 가 같은 클러스터 안에 있으면 kubernetes_host 만으로 된다(Vault Pod 의 SA token 으로 TokenReview 를 호출한다)
vault_exec write auth/kubernetes/config kubernetes_host=https://kubernetes.default.svc

vault_exec write auth/kubernetes/role/member-api \
  bound_service_account_names=member-api \
  bound_service_account_namespaces=member-api \
  token_policies=member-api \
  token_ttl=1h

vault_exec write auth/kubernetes/role/member-api-migration \
  bound_service_account_names=member-api-migration \
  bound_service_account_namespaces=member-api \
  token_policies=member-api-migration \
  token_ttl=10m

# VSO 가 VaultStaticSecret 과 같은 namespace 의 SA 로 로그인한다(3편)
vault_exec write auth/kubernetes/role/vso-member-api \
  bound_service_account_names=vso-member-api \
  bound_service_account_namespaces=member-api \
  token_policies=vso-member-api \
  token_ttl=1h

audits="$(vault_exec audit list -format=json 2>/dev/null || echo '{}')"
if ! jq -e 'has("file/")' <<<"$audits" > /dev/null; then
  vault_exec audit enable file file_path=stdout
fi

echo "vault setup done"
