# ApplicationSet 패턴

ApplicationSet은 ArgoCD에서 여러 Application을 자동으로 생성하고 관리하는 강력한 기능입니다.

## 파일 설명

### 1. appset.yaml (Git Generator)
**자동 탐지 방식** - `chart/` 폴더의 모든 서브디렉토리를 자동으로 탐지하여 Application 생성

```bash
kubectl apply -f bootstrap/application-set/appset.yaml
```

**장점:**
- chart 폴더에 새 차트를 추가하면 자동으로 Application 생성
- 별도의 설정 변경 없이 확장 가능
- DRY (Don't Repeat Yourself) 원칙 준수

**생성되는 Application:**
- `echo-server`
- `hello-world-server`
- `hello-world-server-hook`

### 2. appset-list.yaml (List Generator)
**명시적 목록 방식** - Application 목록을 명시적으로 정의

```bash
kubectl apply -f bootstrap/application-set/appset-list.yaml
```

**장점:**
- 각 Application별로 세밀한 설정 가능 (namespace, 기타 파라미터)
- 명확한 제어와 가독성
- 특정 Application만 선택적으로 배포 가능

**사용 예시:**
- Application별로 다른 namespace 사용
- Application별로 다른 설정 적용

### 3. appset-matrix.yaml (Matrix Generator)
**다중 환경 배포 방식** - 여러 환경(dev, staging, prod)에 동일한 애플리케이션 배포

```bash
kubectl apply -f bootstrap/application-set/appset-matrix.yaml
```

**장점:**
- 여러 환경에 동일한 애플리케이션을 일관되게 배포
- 환경별로 다른 설정 적용 (replica 수, resource 등)
- 환경 추가/제거가 용이

**생성되는 Application (예시):**
- `echo-server-dev`, `echo-server-staging`, `echo-server-prod`
- `hello-world-server-dev`, `hello-world-server-staging`, `hello-world-server-prod`
- `hello-world-server-hook-dev`, `hello-world-server-hook-staging`, `hello-world-server-hook-prod`

## ApplicationSet vs App of Apps

| 특성 | ApplicationSet | App of Apps |
|------|----------------|-------------|
| **자동화** | 높음 (자동 탐지 가능) | 수동 (파일 추가 필요) |
| **동적 생성** | ✅ Generator 패턴 사용 | ❌ 정적 파일 기반 |
| **다중 환경** | ✅ Matrix Generator로 쉽게 구현 | ❌ 파일 복제 필요 |
| **학습 곡선** | 중간 | 낮음 |
| **유연성** | 매우 높음 | 중간 |
| **디버깅** | 중간 | 쉬움 |

## Generator 유형

ApplicationSet은 다양한 Generator를 제공합니다:

1. **List Generator**: 명시적 목록
2. **Git Generator**: Git 저장소 구조 기반
3. **Matrix Generator**: 여러 Generator 조합
4. **Cluster Generator**: 여러 클러스터에 배포
5. **Pull Request Generator**: PR 기반 미리보기 환경
6. **SCM Provider Generator**: GitHub/GitLab 조직의 모든 저장소

## 권장 사용 방법

- **간단한 프로젝트**: `appset.yaml` (Git Generator)
- **세밀한 제어 필요**: `appset-list.yaml` (List Generator)
- **다중 환경**: `appset-matrix.yaml` (Matrix Generator)
- **초보자**: App of Apps 패턴부터 시작

## 확인 방법

ApplicationSet 생성 후 자동으로 생성된 Application 확인:

```bash
# ApplicationSet 확인
kubectl get applicationset -n argocd

# 생성된 Application 확인
kubectl get application -n argocd
```

