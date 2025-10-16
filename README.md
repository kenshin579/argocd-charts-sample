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

