# Docker Desktop → kind 전환 설계

작성일: 2026-06-21

## 배경 / 목적

`argocd-charts-sample`은 현재 **Docker Desktop의 Kubernetes**를 전제로 동작한다.
Docker Desktop은 GUI에서 Settings > Kubernetes > Enable을 눌러야 클러스터가 뜨므로,
클러스터 생성 단계가 터미널 밖(GUI)에 묶여 있다.

목표는 **클러스터 생성·접근·삭제를 전부 터미널 CLI로** 수행하는 것이며,
이를 위해 로컬 클러스터를 **kind**로 전환한다.

## 결정 사항

| 항목 | 결정 |
|---|---|
| 클러스터 도구 | kind |
| 토폴로지 | 단일 노드 (control-plane 1개) |
| 클러스터명 / context | `argo-cluster` / `kind-argo-cluster` (kind가 `kind-` 접두사 자동 부여) |
| kind config | 저장소에 커밋 (`terraform/kind-config.yaml`) |
| ArgoCD UI 접근 | `kubectl port-forward` (kubevpn 제거) |
| ArgoCD helm 설치 | 변경 없음 (ClusterIP 유지, port-forward로 충분) |

## 사전 발견 사항

### `.gitignore` 과대 패턴

현재 `.gitignore`에 `terraform*`, `.terraform*` 패턴이 있어
신규 파일 `terraform/kind-config.yaml`이 git에 무시된다 (`git check-ignore`로 확인).
기존 `.tf` 파일은 이미 추적 중이라 살아있을 뿐이다.
→ 패턴을 state/lock/디렉토리만 무시하도록 좁혀 소스 yaml이 추적되게 한다.

### 블로그 영향 없음

`blog-v2.advenoh.pe.kr`의 글들을 점검한 결과, 이번 전환으로 깨지는 콘텐츠는 없다.

- kubevpn 글(`포트포워딩-없이-...-kubevpn으로-네트워크-연결`)은 **Minikube + bookinfo 예제**로,
  `argocd-charts-sample`과 무관하다.
- argocd 글들은 `repoURL: github.com/kenshin579/argocd-charts-sample`(GitHub URL)와
  ArgoCD manifest YAML만 인용한다. 로컬 클러스터 종류와 무관하므로 변경 불필요.

## 변경 파일 목록

| 파일 | 변경 내용 |
|---|---|
| `terraform/kind-config.yaml` (신규) | 단일 노드 kind 설정 (`name: argo-cluster`, control-plane 1개) |
| `.gitignore` | `terraform*` / `.terraform*` 과대 패턴을 `.terraform/`, `*.tfstate*`, `.terraform.lock.hcl`로 축소 |
| `terraform/variables.tf` | `kube_context` 변수 추가 (기본값 `kind-argo-cluster`) |
| `terraform/main.tf` | kubernetes/helm provider의 `config_context = "docker-desktop"` → `var.kube_context` |
| `terraform/k8s.tf` | `kubectl config set-context docker-desktop ...` → `... ${var.kube_context} ...` |
| `terraform/outputs.tf` | 주석을 kind 기준으로 갱신 |
| `terraform/Makefile` | `kind-create`/`kind-delete` 추가, `all`에 `kind-create` 선행, `view-argocd-password`·`argocd-port-forward` 추가, kubevpn 타깃 제거 |
| `README.md` | "1. Docker Desktop 설정" → "kind 클러스터 생성", "4. UI 접속"의 kubevpn → port-forward |
| `CLAUDE.md` (프로젝트) | "Docker Desktop Kubernetes" → "kind" |
| `../CLAUDE.md` (워크스페이스 루트) | argocd-charts-sample 설명 줄의 "Docker Desktop Kubernetes" → "kind" |

## 상세 설계

### terraform/kind-config.yaml (신규)

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: argo-cluster
nodes:
  - role: control-plane
```

port-forward로 접근하므로 `extraPortMappings`는 불필요. 단일 노드 최소 구성.

### .gitignore

`terraform*`, `.terraform*` 두 줄을 다음으로 교체:

```
.terraform/
*.tfstate
*.tfstate.*
.terraform.lock.hcl
*.tfvars
```

state/lock은 계속 무시, 소스(`.tf`, `kind-config.yaml`)는 추적.

### terraform/variables.tf

```hcl
variable "kube_context" {
  type    = string
  default = "kind-argo-cluster"
}
```

기존 `kubeconfig` 변수는 유지.

### terraform/main.tf

두 provider 모두 `config_context = "docker-desktop"`을 `config_context = var.kube_context`로 변경.

### terraform/k8s.tf

`null_resource.set_default_namespace`의 local-exec 명령에서 하드코딩된 `docker-desktop`을
`${var.kube_context}`로 치환.

### terraform/Makefile

```makefile
CLUSTER_NAME := argo-cluster

.PHONY: kind-create
kind-create:
	@kind get clusters | grep -q '^argo-cluster$$' || kind create cluster --config kind-config.yaml

.PHONY: kind-delete
kind-delete:
	@kind delete cluster --name $(CLUSTER_NAME)

.PHONY: all
all: kind-create tf-init tf-infra

.PHONY: view-argocd-password
view-argocd-password:
	@kubectl -n argocd get secret argocd-secret -o jsonpath="{.data.admin\.password}" | base64 -d; echo

.PHONY: argocd-port-forward
argocd-port-forward:
	@kubectl port-forward svc/argocd-server -n argocd 8080:443
```

- `setup-kubevpn` / `clean-kubevpn` 타깃 제거.
- `tf-clean`의 `clean-kubevpn` 의존 제거 (terraform state 정리만 수행).

## 전환 후 워크플로우 (전부 터미널)

```bash
cd terraform
make all                   # kind 클러스터 생성 + ArgoCD 설치
make view-argocd-password  # 초기 admin 비번 확인
make argocd-port-forward   # 별도 터미널에서 실행 → https://localhost:8080
# ... ApplicationSet 등록 등 기존 절차 동일 ...
make kind-delete           # 정리
```

## 비목표 (YAGNI)

- 멀티 노드 / worker 추가 (단일 노드로 충분)
- Ingress / NodePort / extraPortMappings (port-forward로 충분)
- ArgoCD `--insecure` 설정 (port-forward + 자체서명 인증서 경고 수용)
- kubevpn 관련 기능 보존 (접근 방식이 port-forward로 대체됨)
