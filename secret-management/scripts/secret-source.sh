#!/usr/bin/env bash
# 설정 시크릿 출처를 바꾼다(SOURCE=sealed|vault). 바꾸기 전에 상태를 확인하고, 위험하면 멈춘다.
set -euo pipefail
source "$(dirname "$0")/lib.sh"
require_context

source="${1:-}"
case "$source" in
  vault)
    # SealedSecret 이 아직 Secret 을 소유하고 있으면, ArgoCD 가 SealedSecret 을 prune 할 때 Secret 이 GC 로 지워진다
    owners="$(k -n member-api get secret member-api-config -o jsonpath='{.metadata.ownerReferences[*].kind}' 2>/dev/null || true)"
    if grep -qw SealedSecret <<<"$owners"; then
      echo "error: Secret member-api-config 를 아직 SealedSecret 이 소유하고 있다. 먼저 SKIP_OWNER_REFS=1 make seal → commit·push → make app-sync 로 소유 관계를 끊는다." >&2
      exit 1
    fi
    ;;
  sealed)
    # 다시 봉인한 SealedSecret 을 ArgoCD 가 아직 못 봤다면 옛 값으로 돌아간다
    local_head="$(git -C "$SM_DIR" rev-parse HEAD)"
    remote_head="$(git -C "$SM_DIR" rev-parse '@{u}' 2>/dev/null || echo none)"
    synced="$(k -n argocd get application member-api -o jsonpath='{.status.sync.revision}')"
    if [ "$local_head" != "$remote_head" ] || [ "$synced" != "$local_head" ]; then
      echo "error: 로컬 HEAD($local_head)를 push 하고 ArgoCD 가 그 revision 을 보고 있어야 한다(지금 ArgoCD=$synced, 원격=$remote_head)." >&2
      exit 1
    fi
    ;;
  *)
    echo "usage: make secret-source SOURCE=sealed|vault" >&2
    exit 1
    ;;
esac

"$(dirname "$0")/set-param.sh" secretSource "$source"
