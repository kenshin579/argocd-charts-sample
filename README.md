# ArgoCD Charts Sample

여기에서 작성한 예제는 아래 링크에서 fork해서 수정한 버전이다

https://github.com/RinkiyaKeDad/gitops-sample

## ArgoCD 설치 및 설정

### 1. Docker Desktop에서 Kubernetes 설정

1. Docker Desktop을 실행합니다
2. 설정(Settings) > Kubernetes로 이동합니다
3. "Enable Kubernetes" 체크박스를 선택합니다
4. "Apply & Restart" 버튼을 클릭하여 Kubernetes를 활성화합니다
5. 설치가 완료되면 좌측 하단에 Kubernetes 상태가 "running"으로 표시됩니다

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

ArgoCD UI에 접속하려면 kubevpn을 사용합니다:

```bash
# Kubernetes context를 argocd로 변경
kubectl config set-context --current --namespace=argocd

# kubevpn 연결
kubevpn connect
```

kubevpn 연결 후 브라우저에서 다음 주소로 접속합니다:

```
https://argocd-server.argocd.svc.cluster.local/
```

#### Admin 비밀번호 확인

초기 admin 비밀번호는 다음 명령어로 확인할 수 있습니다:

```bash
kubectl get secret argocd-secret -n argocd -o jsonpath="{.data.admin\.password}" | base64 -d
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

