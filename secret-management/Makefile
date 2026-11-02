# Vault + Agent Injector 실습. 모든 명령은 kind-argo-cluster context 로만 보낸다.
KCTX ?= kind-argo-cluster
export KCTX
KUBECTL := kubectl --context $(KCTX)

.PHONY: cluster check-context bootstrap vault-init vault-unseal vault-status vault-setup

## kind 클러스터와 ArgoCD 설치 (../terraform)
## 로컬 helm repo 설정에 접속 안 되는 repo 가 있으면 terraform helm provider 가 실패하므로 빈 설정으로 실행한다
HELM_ISOLATED := HELM_REPOSITORY_CONFIG=$(CURDIR)/.helm/repositories.yaml HELM_REPOSITORY_CACHE=$(CURDIR)/.helm/cache
cluster:
	@mkdir -p .helm/cache
	@printf 'apiVersion: ""\nrepositories: []\n' > .helm/repositories.yaml
	$(HELM_ISOLATED) $(MAKE) -C ../terraform all

check-context:
	@bash -c 'source scripts/lib.sh && require_context'

## sealed-secrets, Vault Application 적용
bootstrap: check-context
	$(KUBECTL) apply -n argocd -f bootstrap/sealed-secrets.yaml -f bootstrap/vault.yaml

vault-init:
	./scripts/vault-init.sh

vault-unseal:
	./scripts/vault-unseal.sh

vault-status:
	./scripts/vault-status.sh

vault-setup:
	./scripts/vault-setup.sh
