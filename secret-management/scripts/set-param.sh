#!/usr/bin/env bash
# member-api Application 의 helm parameter 하나를 추가하거나 바꾼다. 다른 parameter 는 그대로 둔다.
# (merge patch 는 배열을 통째로 바꾸므로, 현재 배열을 읽어 고친 뒤 전체를 다시 넣는다)
set -euo pipefail
source "$(dirname "$0")/lib.sh"
require_context

name="${1:?usage: set-param.sh NAME VALUE}"
value="${2:?usage: set-param.sh NAME VALUE}"

current="$(k -n argocd get application member-api -o json | jq '.spec.source.helm.parameters // []')"
params="$(jq -c --arg n "$name" --arg v "$value" '[.[] | select(.name != $n)] + [{name: $n, value: $v}]' <<<"$current")"
k -n argocd patch application member-api --type merge \
  -p "{\"spec\":{\"source\":{\"helm\":{\"parameters\":$params}}}}" > /dev/null
echo "helm parameters: $params"
