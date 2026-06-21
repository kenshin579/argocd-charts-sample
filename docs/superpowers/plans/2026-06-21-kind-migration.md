# Docker Desktop → kind 전환 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `argocd-charts-sample`의 로컬 K8s 환경을 Docker Desktop(GUI)에서 kind(CLI)로 전환해, 클러스터 생성·접근·삭제를 전부 터미널에서 수행한다.

**Architecture:** 단일 노드 kind 클러스터(`argo-cluster`, context `kind-argo-cluster`)를 `terraform/kind-config.yaml`로 정의한다. Terraform provider는 하드코딩된 `docker-desktop` context 대신 `var.kube_context`를 참조한다. ArgoCD는 기존 ClusterIP 설치를 유지하고 접근은 `kubectl port-forward`로 단순화한다(kubevpn 제거). Makefile이 클러스터 생성부터 ArgoCD 설치·접근·정리까지의 CLI 진입점을 제공한다.

**Tech Stack:** kind, Terraform (kubernetes/helm provider), Helm (argo-cd), kubectl, Make

> **검증 방식 주의:** 이 플랜은 인프라 설정 변경이라 코드 unit test 대신 **명령 실행 + 출력 검증**으로 각 변경을 확인한다. 실제 클러스터를 띄우는 통합 검증은 마지막 Task에 모아둔다 (Task 1~7은 파일 변경 + 정적 검증, Task 8에서 end-to-end 실행).

**전제:** `kind v0.31.0`, `kubectl`, `terraform` 모두 설치 확인됨. 작업 브랜치 `chore/kind-migration` (spec 커밋 fda5c3b 위에서 진행).

---

## File Structure

| 파일 | 책임 | 작업 |
|---|---|---|
| `terraform/kind-config.yaml` | 단일 노드 kind 클러스터 정의 | Create |
| `.gitignore` | terraform state/lock만 무시, 소스는 추적 | Modify |
| `terraform/variables.tf` | `kube_context` 변수 정의 | Modify |
| `terraform/main.tf` | provider가 `var.kube_context` 참조 | Modify |
| `terraform/k8s.tf` | default namespace 설정 시 kind context 사용 | Modify |
| `terraform/outputs.tf` | 주석을 kind 기준으로 갱신 | Modify |
| `terraform/Makefile` | kind 생성/삭제·ArgoCD 접근 CLI 진입점 | Modify |
| `README.md` | 설치/접근 절차를 kind+port-forward로 갱신 | Modify |
| `CLAUDE.md` | 프로젝트 설명을 kind로 갱신 | Modify |
| `../CLAUDE.md` (워크스페이스 루트) | argocd-charts-sample 설명·명령을 kind로 갱신 | Modify |

---

## Task 1: kind 클러스터 정의 파일 생성

**Files:**
- Create: `terraform/kind-config.yaml`

- [ ] **Step 1: kind-config.yaml 작성**

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: argo-cluster
nodes:
  - role: control-plane
```

- [ ] **Step 2: kind 설정 유효성 검증 (클러스터 실제 생성 없이 dry-run)**

Run:
```bash
cd terraform && kind create cluster --config kind-config.yaml --dry-run
```
Expected: 에러 없이 클러스터 생성 계획이 출력됨 (`Creating cluster "argo-cluster" ...` 류). 에러로 끝나지 않으면 OK.

> 참고: `--dry-run` 플래그가 동작하지 않는 kind 버전이면 YAML 문법만 `kubectl --validate` 불가하므로, 이 단계는 "Task 8에서 실제 생성으로 검증"으로 대체한다. v0.31.0은 dry-run 지원.

- [ ] **Step 3: Commit**

```bash
git add terraform/kind-config.yaml
git commit -m "[#kind] feat: 단일 노드 kind 클러스터 정의 추가"
```

> ⚠️ 다음 Task에서 `.gitignore`를 고치기 전까지 이 파일이 무시될 수 있다. `git add` 후 `git status`로 staged 되었는지 확인할 것. staged 되지 않으면 Task 2를 먼저 수행한 뒤 다시 add.

---

## Task 2: .gitignore 과대 패턴 축소

**Files:**
- Modify: `.gitignore`

**배경:** 현재 `terraform*`, `.terraform*` 패턴이 `terraform/kind-config.yaml`까지 무시한다 (`git check-ignore -v terraform/kind-config.yaml` → 매치 확인됨). state/lock/디렉토리만 무시하도록 좁힌다.

- [ ] **Step 1: 변경 전 무시 여부 확인**

Run: `git check-ignore -v terraform/kind-config.yaml`
Expected: `.gitignore:20:terraform*	terraform/kind-config.yaml` (무시되고 있음)

- [ ] **Step 2: .gitignore에서 마지막 두 줄 교체**

`.gitignore` 끝부분의 아래 두 줄을:
```
.terraform*
terraform*
```
다음으로 교체:
```
.terraform/
*.tfstate
*.tfstate.*
.terraform.lock.hcl
*.tfvars
```

- [ ] **Step 3: 변경 후 무시 해제 확인**

Run: `git check-ignore -v terraform/kind-config.yaml; echo "exit=$?"`
Expected: 출력 없음, `exit=1` (더 이상 무시되지 않음)

- [ ] **Step 4: 기존 state 파일은 여전히 무시되는지 확인 (회귀 방지)**

Run: `git check-ignore terraform/terraform.tfstate terraform/.terraform.lock.hcl`
Expected: 두 경로 모두 출력됨 (계속 무시됨)

- [ ] **Step 5: kind-config.yaml이 아직 커밋 안 됐으면 함께 add**

Run: `git add .gitignore terraform/kind-config.yaml && git status --short`
Expected: `.gitignore`와 `terraform/kind-config.yaml`이 staged 상태로 표시

- [ ] **Step 6: Commit**

```bash
git commit -m "[#kind] fix: .gitignore 과대 패턴 축소로 kind-config 추적 가능하게"
```

---

## Task 3: terraform 변수에 kube_context 추가

**Files:**
- Modify: `terraform/variables.tf`

- [ ] **Step 1: kube_context 변수 추가**

`terraform/variables.tf` 끝에 추가 (기존 `argotest_namespace`, `kubeconfig` 변수는 유지):

```hcl
variable "kube_context" {
  type    = string
  default = "kind-argo-cluster"
}
```

- [ ] **Step 2: Commit**

```bash
git add terraform/variables.tf
git commit -m "[#kind] feat: kube_context 변수 추가 (기본값 kind-argo-cluster)"
```

---

## Task 4: provider context를 변수로 전환

**Files:**
- Modify: `terraform/main.tf`

- [ ] **Step 1: 두 provider의 config_context를 변수 참조로 변경**

`terraform/main.tf`에서 `kubernetes` provider와 `helm` provider 안의
`config_context = "docker-desktop"`을 **두 곳 모두** 다음으로 변경:

```hcl
config_context = var.kube_context
```

변경 후 provider 블록 모습:

```hcl
provider "kubernetes" {
  config_path    = var.kubeconfig
  config_context = var.kube_context
}

provider "helm" {
  kubernetes = {
    config_path    = var.kubeconfig
    config_context = var.kube_context
  }
}
```

- [ ] **Step 2: terraform fmt + validate**

Run:
```bash
cd terraform && terraform fmt && terraform init -backend=false -upgrade >/dev/null && terraform validate
```
Expected: `Success! The configuration is valid.`

- [ ] **Step 3: Commit**

```bash
git add terraform/main.tf
git commit -m "[#kind] refactor: provider context를 docker-desktop → var.kube_context"
```

---

## Task 5: k8s.tf의 하드코딩 context 제거

**Files:**
- Modify: `terraform/k8s.tf`

- [ ] **Step 1: local-exec 명령의 하드코딩 context 치환**

`terraform/k8s.tf`의 `null_resource.set_default_namespace` 안에서:

```hcl
    command = "kubectl config set-context docker-desktop --namespace=${var.argotest_namespace}"
```
를 다음으로 변경:

```hcl
    command = "kubectl config set-context ${var.kube_context} --namespace=${var.argotest_namespace}"
```

- [ ] **Step 2: terraform fmt + validate**

Run: `cd terraform && terraform fmt && terraform validate`
Expected: `Success! The configuration is valid.`

- [ ] **Step 3: Commit**

```bash
git add terraform/k8s.tf
git commit -m "[#kind] refactor: set-context의 하드코딩 docker-desktop 제거"
```

---

## Task 6: outputs.tf 주석 갱신

**Files:**
- Modify: `terraform/outputs.tf`

- [ ] **Step 1: 주석 교체**

`terraform/outputs.tf`의 기존 Docker Desktop 관련 주석 전체를 다음으로 교체:

```hcl
# kind 클러스터(argo-cluster, context kind-argo-cluster)를 사용한다.
# kubeconfig는 `kind create cluster` 시 ~/.kube/config에 자동 병합되므로
# 별도의 kubeconfig 출력이 필요하지 않다.
```

- [ ] **Step 2: Commit**

```bash
git add terraform/outputs.tf
git commit -m "[#kind] docs: outputs.tf 주석을 kind 기준으로 갱신"
```

---

## Task 7: Makefile에 kind 라이프사이클 + ArgoCD 접근 타깃 추가

**Files:**
- Modify: `terraform/Makefile`

- [ ] **Step 1: 상단에 CLUSTER_NAME 변수 추가, kubevpn 함수 블록 제거**

`terraform/Makefile` 상단의 `define KUBEVPN_CMD ... endef` 블록을 삭제하고,
`NAMESPACE := argocd-test` 아래에 추가:

```makefile
CLUSTER_NAME := argo-cluster
```

- [ ] **Step 2: kind-create / kind-delete 타깃 추가**

`.PHONY: all` 위에 추가:

```makefile
.PHONY: kind-create
kind-create:
	@echo "Creating kind cluster ($(CLUSTER_NAME)) if not exists..."
	@kind get clusters | grep -q '^$(CLUSTER_NAME)$$' || kind create cluster --config kind-config.yaml

.PHONY: kind-delete
kind-delete:
	@echo "Deleting kind cluster ($(CLUSTER_NAME))..."
	@kind delete cluster --name $(CLUSTER_NAME)
```

- [ ] **Step 3: all 타깃이 kind-create를 선행하도록 변경**

```makefile
.PHONY: all
all: kind-create tf-init tf-infra
```

- [ ] **Step 4: ArgoCD 접근 타깃 추가**

```makefile
.PHONY: view-argocd-password
view-argocd-password:
	@kubectl -n argocd get secret argocd-secret -o jsonpath="{.data.admin\.password}" | base64 -d; echo

.PHONY: argocd-port-forward
argocd-port-forward:
	@echo "ArgoCD UI → https://localhost:8080 (Ctrl-C로 종료)"
	@kubectl port-forward svc/argocd-server -n argocd 8080:443
```

- [ ] **Step 5: kubevpn 타깃 제거 및 tf-clean 의존 정리**

`setup-kubevpn`, `clean-kubevpn` 타깃을 삭제한다.
`tf-clean: clean-kubevpn`을 다음으로 변경 (kubevpn 의존 제거):

```makefile
.PHONY: tf-clean
tf-clean:
	@echo "Force cleaning Terraform state..."
	@rm -rf .terraform/
	@rm -f .terraform.lock.hcl
	@rm -f terraform.tfstate
	@rm -f terraform.tfstate.backup
	@echo "Terraform cleanup completed."
```

- [ ] **Step 6: Makefile 문법 검증 (타깃 목록 확인)**

Run:
```bash
cd terraform && make -n kind-create && make -n view-argocd-password && make -n argocd-port-forward
```
Expected: 각 타깃의 명령이 에러 없이 출력됨. `kubevpn` 문자열이 더 이상 등장하지 않음.

Run: `grep -c kubevpn Makefile; echo "exit=$?"`
Expected: `0` (kubevpn 잔재 없음)

- [ ] **Step 7: Commit**

```bash
git add terraform/Makefile
git commit -m "[#kind] feat: Makefile에 kind 라이프사이클·ArgoCD 접근 타깃 추가, kubevpn 제거"
```

---

## Task 8: 문서 갱신 (README + CLAUDE.md x2)

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `../CLAUDE.md` (워크스페이스 루트)

- [ ] **Step 1: README.md "1. Docker Desktop에서 Kubernetes 설정" 섹션 교체**

해당 섹션 전체(번호 목록 포함)를 다음으로 교체:

```markdown
### 1. kind로 Kubernetes 클러스터 생성

GUI 없이 터미널에서 단일 노드 클러스터를 생성합니다 (`terraform/kind-config.yaml` 사용):

```bash
cd terraform
make kind-create
```

생성되는 클러스터명은 `argo-cluster`이며, kubectl context는 `kind-argo-cluster`로 등록됩니다.
```

- [ ] **Step 2: README.md "4. ArgoCD UI 접속" 섹션을 port-forward로 교체**

kubevpn 안내 블록(context 변경 + `kubevpn connect` + `cluster.local` URL)을 다음으로 교체:

```markdown
### 4. ArgoCD UI 접속

`kubectl port-forward`로 로컬에서 접속합니다 (별도 터미널에서 실행):

```bash
cd terraform
make argocd-port-forward
```

브라우저에서 `https://localhost:8080` 으로 접속합니다 (자체 서명 인증서 경고는 무시).

#### Admin 비밀번호 확인

```bash
cd terraform
make view-argocd-password
```
```

> 기존 "#### Admin 비밀번호 확인"의 `kubectl get secret ...` raw 명령 블록은 위 `make view-argocd-password`로 대체되므로 중복되면 제거한다.

- [ ] **Step 3: README 정리 — 클러스터 삭제 안내 추가 (Cleanup 근처 또는 4번 섹션 뒤)**

```markdown
### 클러스터 삭제

```bash
cd terraform
make kind-delete
```
```

- [ ] **Step 4: 프로젝트 CLAUDE.md line 7 갱신**

`CLAUDE.md`의 "Docker Desktop Kubernetes에서 ArgoCD를 설치하고"를
"kind 클러스터(argo-cluster)에서 ArgoCD를 설치하고"로 변경.

- [ ] **Step 5: 워크스페이스 루트 ../CLAUDE.md 갱신 (2곳)**

`../CLAUDE.md` line 260:
`- 환경: Docker Desktop Kubernetes에 Terraform(`terraform/`)으로 ArgoCD 설치`
→ `- 환경: kind 클러스터(argo-cluster)에 Terraform(`terraform/`)으로 ArgoCD 설치`

`../CLAUDE.md`의 "### ArgoCD Charts Sample" Quick Commands 블록(line 117~)에 kind 라이프사이클 명령 추가. `make tf-infra` 줄 위에 삽입:
```
make kind-create     # kind 클러스터 생성 (argo-cluster)
```
그리고 `make tf-destroy` 줄 아래에 삽입:
```
make kind-delete     # kind 클러스터 삭제
make argocd-port-forward   # ArgoCD UI (https://localhost:8080)
make view-argocd-password  # admin 초기 비번
```

- [ ] **Step 6: docker-desktop / kubevpn 잔재 grep 검증**

Run:
```bash
cd /Users/user/src/workspace_blogv2/argocd-charts-sample && grep -rniE "docker[- ]desktop" README.md CLAUDE.md terraform/ ; echo "exit=$?"
```
Expected: 출력 없음, `exit=1` (잔재 없음)

- [ ] **Step 7: Commit**

```bash
cd /Users/user/src/workspace_blogv2/argocd-charts-sample
git add README.md CLAUDE.md ../CLAUDE.md
git commit -m "[#kind] docs: README·CLAUDE.md를 kind+port-forward 기준으로 갱신"
```

---

## Task 9: End-to-end 통합 검증 (실제 클러스터)

**Files:** (변경 없음 — 실행 검증만)

> 이 Task는 실제로 kind 클러스터를 띄우고 ArgoCD가 올라오는지 확인한다. 시간이 걸리고(수 분) 로컬 리소스를 사용한다. 검증 후 정리한다.

- [ ] **Step 1: 클러스터 생성 + ArgoCD 설치 한 번에**

Run: `cd terraform && make all`
Expected: kind 클러스터 `argo-cluster` 생성 → `terraform apply` 성공 → ArgoCD helm release 설치 완료. 에러 없이 종료.

- [ ] **Step 2: context 확인**

Run: `kubectl config current-context`
Expected: `kind-argo-cluster`

- [ ] **Step 3: ArgoCD 파드 Running 확인**

Run: `kubectl get pods -n argocd`
Expected: `argocd-server`, `argocd-repo-server`, `argocd-application-controller` 등이 `Running` 상태 (초기엔 일부 ContainerCreating일 수 있으니 1~2분 대기 후 재확인)

- [ ] **Step 4: admin 비번 출력 확인**

Run: `make view-argocd-password`
Expected: 비어있지 않은 비밀번호 문자열 1줄 출력

- [ ] **Step 5: port-forward 접근 확인 (백그라운드)**

Run:
```bash
kubectl port-forward svc/argocd-server -n argocd 8080:443 >/tmp/argocd-pf.log 2>&1 &
PF_PID=$!
sleep 3
curl -sk https://localhost:8080/healthz; echo
kill $PF_PID
```
Expected: `ok` (또는 200 응답). ArgoCD server에 로컬 접근 가능 확인.

- [ ] **Step 6: ApplicationSet 등록 동작 확인 (기존 절차 회귀 테스트)**

Run:
```bash
cd /Users/user/src/workspace_blogv2/argocd-charts-sample
kubectl apply -f bootstrap/application-set/appset-list.yaml -n argocd
kubectl get applicationset -n argocd
```
Expected: ApplicationSet이 생성되고 조회됨 (기존 docker-desktop 시절과 동일하게 동작).

- [ ] **Step 7: 정리**

Run: `cd terraform && make kind-delete`
Expected: `Deleting cluster "argo-cluster" ...` 후 정상 삭제.

- [ ] **Step 8: 통합 검증 결과를 커밋 메시지 없이 보고**

검증만 수행하므로 커밋 없음. Step 1~7 결과(특히 Step 2 context, Step 3 파드, Step 5 healthz)를 사용자에게 보고한다.

---

## Self-Review 결과

- **Spec coverage:** spec의 변경 파일 목록 9개 + 워크플로우 → Task 1~8이 전부 커버, Task 9가 워크플로우 검증. ✅
- **Placeholder scan:** TBD/TODO/"적절히 처리" 없음. 모든 코드 블록 실내용 포함. ✅
- **Type consistency:** 변수명 `kube_context`(Task 3 정의 → Task 4·5 사용), context `kind-argo-cluster`, 클러스터명 `argo-cluster`, 타깃명 `kind-create`/`kind-delete`/`view-argocd-password`/`argocd-port-forward` 전 Task 일관. ✅
- **순서 주의:** Task 1 커밋이 Task 2(.gitignore)에 의존 → Task 1 Step 3에 경고 명시, Task 2 Step 5에서 함께 add 처리.
