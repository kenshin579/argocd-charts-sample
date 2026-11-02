# Vault + Agent Injector 실습. 모든 명령은 kind-argo-cluster context 로만 보낸다.
KCTX ?= kind-argo-cluster
export KCTX
KUBECTL := kubectl --context $(KCTX)
KIND_CLUSTER ?= argo-cluster
APP_IMAGE ?= member-api:0.1.0
REPO ?= https://github.com/kenshin579/argocd-charts-sample
REVISION ?= HEAD
MODE ?= async

.PHONY: cluster check-context bootstrap vault-init vault-unseal vault-status vault-setup app-image seal app-deploy app-port-forward load-mode app-sync init-dek-logs vault-seal snapshot-save snapshot-restore

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

## 데모용 seal (Vault 재시작과 같은 상태로 만든다)
vault-seal:
	./scripts/vault-seal.sh

snapshot-save:
	./scripts/snapshot.sh save

## make snapshot-restore SNAPSHOT=.vault/snapshots/vault-<시각>.snap
snapshot-restore:
	./scripts/snapshot.sh restore $(SNAPSHOT)

vault-setup:
	./scripts/vault-setup.sh

## member-api 이미지를 빌드해 kind 노드에 넣는다 (latest 태그는 pull policy 가 Always 가 되므로 쓰지 않는다)
app-image: check-context
	docker build -t $(APP_IMAGE) app/member-api
	kind load docker-image $(APP_IMAGE) --name $(KIND_CLUSTER)

## 설정 시크릿을 SealedSecret 으로 봉인한다 (charts/member-api/templates/sealedsecret.yaml)
seal:
	./scripts/seal.sh

## member-api Application 적용. ArgoCD 는 GitHub 에서 chart 를 가져온다.
## fork 해서 실습할 때는 make seal 결과를 commit·push 한 뒤 REPO=<fork URL> REVISION=<브랜치> 로 지정한다
app-deploy: check-context
	sed -e 's|repoURL: https://github.com/kenshin579/argocd-charts-sample|repoURL: $(REPO)|' \
	    -e 's|targetRevision: HEAD|targetRevision: $(REVISION)|' bootstrap/member-api.yaml \
	  | $(KUBECTL) apply -n argocd -f -

## localhost:8081 → member-api Service(80)
app-port-forward: check-context
	$(KUBECTL) -n member-api port-forward svc/member-api 8081:80

## LOAD_MODE 전환 (MODE=sync|async). git 을 바꾸지 않고 Application 의 helm parameter 로 덮어쓴다
load-mode: check-context
	$(KUBECTL) -n argocd patch application member-api --type merge \
	  -p '{"spec":{"source":{"helm":{"parameters":[{"name":"loadMode","value":"$(MODE)"}]}}}}'

## member-api 를 지금 sync 한다(DEK 적재 Job 이 다시 돈다). argocd CLI 없이 Application 에 operation 을 넣는다
app-sync: check-context
	$(KUBECTL) -n argocd patch application member-api --type merge \
	  -p '{"operation":{"initiatedBy":{"username":"make"},"sync":{}}}'

## 가장 최근 DEK 적재(init-dek) Job 의 로그
init-dek-logs: check-context
	@job=$$($(KUBECTL) -n member-api get jobs -l app.kubernetes.io/component=init-dek \
	  --sort-by=.metadata.creationTimestamp -o name | tail -1); \
	  echo "$$job"; $(KUBECTL) -n member-api logs "$$job"
