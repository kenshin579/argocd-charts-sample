# Vault + Agent Injector 실습. 모든 명령은 kind-argo-cluster context 로만 보낸다.
KCTX ?= kind-argo-cluster
export KCTX
KUBECTL := kubectl --context $(KCTX)
KIND_CLUSTER ?= argo-cluster
APP_IMAGE ?= member-api:0.1.0
REVISION ?= HEAD
MODE ?= async

.PHONY: cluster check-context bootstrap vault-init vault-unseal vault-status vault-setup app-image seal app-deploy app-port-forward load-mode

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

## member-api 이미지를 빌드해 kind 노드에 넣는다 (latest 태그는 pull policy 가 Always 가 되므로 쓰지 않는다)
app-image: check-context
	docker build -t $(APP_IMAGE) app/member-api
	kind load docker-image $(APP_IMAGE) --name $(KIND_CLUSTER)

## 설정 시크릿을 SealedSecret 으로 봉인한다 (charts/member-api/templates/sealedsecret.yaml)
seal:
	./scripts/seal.sh

## member-api Application 적용. 개발 중에는 REVISION=<브랜치> 로 GitHub 의 그 브랜치를 보게 한다
app-deploy: check-context
	sed 's|targetRevision: HEAD|targetRevision: $(REVISION)|' bootstrap/member-api.yaml | $(KUBECTL) apply -n argocd -f -

## localhost:8081 → member-api Service(80)
app-port-forward: check-context
	$(KUBECTL) -n member-api port-forward svc/member-api 8081:80

## LOAD_MODE 전환 (MODE=sync|async). git 을 바꾸지 않고 Application 의 helm parameter 로 덮어쓴다
load-mode: check-context
	$(KUBECTL) -n argocd patch application member-api --type merge \
	  -p '{"spec":{"source":{"helm":{"parameters":[{"name":"loadMode","value":"$(MODE)"}]}}}}'
