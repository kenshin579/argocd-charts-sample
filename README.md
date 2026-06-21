# ArgoCD Charts Sample

여기에서 작성한 예제는 아래 링크에서 fork해서 수정한 버전이다

https://github.com/RinkiyaKeDad/gitops-sample

## ArgoCD 설치 및 설정

### 1. kind로 Kubernetes 클러스터 생성

GUI 없이 터미널에서 단일 노드 클러스터를 생성합니다 (`terraform/kind-config.yaml` 사용):

```bash
cd terraform
make kind-create
```

생성되는 클러스터명은 `argo-cluster`이며, kubectl context는 `kind-argo-cluster`로 등록됩니다.

### 2. Terraform으로 ArgoCD 설치

프로젝트의 `terraform` 폴더로 이동하여 다음 명령어를 실행합니다:

```bash
cd terraform
make tf-infra
```

이 명령어는 다음 작업을 수행합니다:
- Kubernetes 클러스터에 ArgoCD 네임스페이스 생성
- ArgoCD 관련 리소스 배포
- 필요한 인프라 구성 요소 설치

### 3. ArgoCD Application 등록

ArgoCD가 설치되면 Application을 등록합니다:

```bash
kubectl apply -f bootstrap/application-set/appset-list.yaml -n argocd
```

이 명령어는 ApplicationSet을 생성하여 여러 애플리케이션을 자동으로 관리합니다.

### 4. ArgoCD UI 접속

`kubectl port-forward`로 로컬에서 접속합니다 (별도 터미널에서 실행):

```bash
cd terraform
make argocd-port-forward
```

브라우저에서 `https://localhost:8080` 으로 접속합니다 (자체 서명 인증서 경고는 무시).

#### Admin 비밀번호

Terraform 변수 `argocd_password`(`terraform/modules/infra/variables.tf`)에는 `argocd account bcrypt --password '<평문>'`으로 생성한 bcrypt 해시를 저장합니다.
로그인 시에는 해시를 만들 때 사용한 `<평문>`을 입력하며, 저장된 해시에서 평문을 되돌려 읽을 수는 없습니다.

저장된 해시 자체를 확인하려면:

```bash
cd terraform
make view-argocd-password
```

### 클러스터 삭제

```bash
cd terraform
make kind-delete
```

## ArgoCD Notifications 설정 (선택)

`argocd-noti-test` namespace의 모든 Application에서 발생하는 **OutOfSync / Sync Failed / Health Degraded** 이벤트를 클러스터 내부 webhook receiver로 알림 받는 로컬 테스트 환경.

### 구성

| 컴포넌트 | namespace | 역할 |
|---|---|---|
| `webhook-receiver` (Deployment + Service) | `argocd-noti-receiver` | webhook을 받아 stdout JSON 출력 |
| `argocd-notifications-cm` / `-secret` | `argocd` | trigger/template/service/subscription 설정 |
| `hello-world-server` (테스트 대상) | `argocd-noti-test` | 알림 발생 대상 (automated 없음 — manual sync) |

### 설치

```bash
# 알림 시스템 부트스트랩 (Application 2개를 한 번에)
kubectl apply -f bootstrap/notifications.yaml

# 알림 테스트 대상 ApplicationSet
kubectl apply -f bootstrap/application-set/appset-noti-test.yaml
```

### 알림 수신 확인

별도 터미널에서 로그 tail:
```bash
kubectl logs -f deployment/webhook-receiver -n argocd-noti-receiver
```

drift 유발 예시:
```bash
# Cluster drift
kubectl scale deployment hello-world-server -n argocd-noti-test --replicas=3
# ~60초 내 OutOfSync 알림 JSON이 webhook-receiver 로그에 출력됨
```

### Cleanup

```bash
kubectl delete -f bootstrap/notifications.yaml
kubectl delete -f bootstrap/application-set/appset-noti-test.yaml
```

자세한 설계는 `docs/superpowers/specs/2026-05-15-argocd-notifications-outofsync-design.md` 참조.

