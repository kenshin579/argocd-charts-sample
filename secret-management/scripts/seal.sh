#!/usr/bin/env bash
# member-api 설정 시크릿(API 토큰)을 kubeseal 로 봉인해 chart 템플릿에 쓴다.
# SealedSecret 은 이 클러스터의 sealed-secrets 컨트롤러 키로만 풀린다. 클러스터를 새로 만들면 다시 실행해 커밋한다.
set -euo pipefail
source "$(dirname "$0")/lib.sh"
require_context

API_TOKEN="$(resolve_api_token)"
out="$SM_DIR/charts/member-api/templates/sealedsecret.yaml"

secret="$(k -n member-api create secret generic member-api-config \
  --from-literal=api-token="$API_TOKEN" --dry-run=client -o yaml)"

# SKIP_OWNER_REFS=1: 컨트롤러가 Secret 에 SealedSecret ownerReference 를 붙이지 않는다(skip-set-owner-references).
# 그러면 SealedSecret 을 지워도 Secret 이 남으므로, 다른 도구(VSO)로 넘길 수 있다(3편).
# ownerReference 가 없으면 컨트롤러는 그 Secret 을 "자기 것이 아니다"로 보고 갱신을 거부하므로,
# managed: "true" 를 함께 달아 계속 관리하게 한다(sealed-secrets README 의 권장 조합)
if [ "${SKIP_OWNER_REFS:-}" = "1" ]; then
  secret="$(sed 's|^metadata:$|metadata:\n  annotations:\n    sealedsecrets.bitnami.com/managed: "true"\n    sealedsecrets.bitnami.com/skip-set-owner-references: "true"|' <<<"$secret")"
fi

sealed="$(kubeseal --context "$KCTX" --format yaml <<<"$secret")"

# 앱(wave 1)보다 먼저 Secret 이 있도록 SealedSecret 을 wave -1 로 둔다
sealed="$(sed 's|^metadata:$|metadata:\n  annotations:\n    argocd.argoproj.io/sync-wave: "-1"|' <<<"$sealed")"

{
  echo '{{- if eq .Values.secretSource "sealed" }}'
  echo "$sealed"
  echo '{{- end }}'
} > "$out"
echo "sealed: $out (api-token sha256=$(sha12 "$API_TOKEN"))"
