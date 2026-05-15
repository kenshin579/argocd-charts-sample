# ArgoCD Notifications 알림 설계 (OutOfSync / Sync Failed / Health Degraded)

**작성일**: 2026-05-15
**대상 레포**: `argocd-charts-sample`
**브랜치**: `feat/argocd-notifications-outofsync`

## 1. 목적

로컬 ArgoCD(Docker Desktop K8s)에서 특정 namespace(`argocd-noti-test`)에 배포된 모든 Application의 주요 lifecycle 이벤트(**OutOfSync**, **Sync Failed**, **Health Degraded**)를 webhook으로 알림 받고, 알림 payload를 `kubectl logs`로 확인할 수 있는 환경을 구성한다.

3개 trigger를 함께 다루는 이유:
- **OutOfSync**: drift 감지 (git ≠ 클러스터 상태) — 변경 추적
- **Sync Failed**: sync 작업 자체가 실패 (manifest 오류, RBAC, helm 오류 등) — 사람 개입 필요
- **Health Degraded**: sync는 정상이지만 Pod 비정상 (CrashLoopBackOff, ImagePullBackOff 등) — 런타임 문제

이 3가지를 묶어야 OutOfSync로 잡지 못하는 영역(런타임 헬스, 복구 시도 실패)까지 커버한다.

## 2. 배경 및 제약사항

- 실제 production 환경에서는 ArgoCD가 이미 운영 중이라 `helm_release` 재배포가 어렵다. 따라서 알림 설정은 기존 ArgoCD 설치와 **분리된 GitOps 자원**으로 관리한다.
- GitOps 원칙을 유지하면서, ArgoCD가 자기 자신의 알림 설정을 sync하는 패턴을 적용한다.
- ArgoCD Notifications 기본 trigger 카탈로그에는 OutOfSync 전용 trigger가 없으므로 커스텀 trigger를 작성한다.
- 현재 ApplicationSet(`appset-list.yaml`)은 `automated.selfHeal: true`라 OutOfSync 상태가 매우 짧다. 의미 있는 알림 테스트를 위해 별도 ApplicationSet에서 `automated`를 완전히 제거한다 (Git drift + Cluster drift 모두 발생 가능).

## 3. 결정 사항 요약

| 항목 | 결정 |
|---|---|
| Notification service | Webhook (외부 의존 없음) |
| Trigger | 3개: 커스텀 `on-sync-status-out-of-sync` + 카탈로그 표준 `on-sync-failed` + `on-health-degraded` |
| 알림 대상 범위 | `argocd-noti-test` namespace에 배포되는 모든 Application |
| Webhook 수신 서버 | `mendhak/http-https-echo` 이미지로 stdout JSON 출력 |
| 알림 설정 통합 방식 | 별도 Helm chart(`chart/argocd-notifications-config/`) + ArgoCD Application으로 sync |
| Subscription 방식 | `argocd-notifications-cm`의 default subscription + 각 trigger 표현식 내 namespace 필터 |
| 테스트 대상 앱 | `hello-world-server` 1개 (단순화) |
| Sync 자동화 | `automated` 완전 제거 (Git drift + Cluster drift, manual sync 실패 등 모두 의미 있게 발생) |

## 4. 아키텍처

### 4.1 전체 구성도

```
┌──────────────────────────────────────────────────────────────────────────┐
│  Kubernetes (Docker Desktop)                                             │
│                                                                          │
│  ┌──────────────────── argocd namespace ────────────────────┐            │
│  │   argo-cd Helm release (기존, 변경 없음)                  │            │
│  │   ├─ argocd-server                                       │            │
│  │   ├─ argocd-application-controller                       │            │
│  │   └─ argocd-notifications-controller                     │            │
│  │                                                          │            │
│  │   argocd-notifications-cm   (Application이 ownership 인수) │            │
│  │   argocd-notifications-secret                            │            │
│  └────────────────────────────────────────────┬─────────────┘            │
│                                               │ POST webhook             │
│                                               ▼                          │
│  ┌─────────── argocd-noti-receiver namespace ───────────┐                │
│  │   webhook-receiver Deployment                        │                │
│  │   (mendhak/http-https-echo)                          │                │
│  │   Service: webhook-receiver:80                       │                │
│  │   → payload를 stdout JSON 출력                       │                │
│  └──────────────────────────────────────────────────────┘                │
│                                                                          │
│  ┌─────────── argocd-noti-test namespace ───────────────┐                │
│  │   hello-world-server (drift 발생 대상)                │                │
│  │   (automated/selfHeal 없음 → manual sync)            │                │
│  └──────────────────────────────────────────────────────┘                │
└──────────────────────────────────────────────────────────────────────────┘
```

### 4.2 namespace 분리

| namespace | 용도 | 기존/신규 |
|---|---|---|
| `argocd` | ArgoCD core (변경 없음) | 기존 |
| `argocd-noti-receiver` | webhook 수신 (격리) | 신규 |
| `argocd-noti-test` | 알림 테스트 대상 앱들 | 신규 |

webhook-receiver를 별도 namespace로 격리하는 이유: 자기 자신이 알림 대상이 되는 것 방지 + 책임 분리.

## 5. 컴포넌트

### 5.1 `chart/argocd-notifications-config/` (신규 Helm chart)

ArgoCD Notifications의 trigger / template / service / default subscription을 정의.

**구조:**
```
chart/argocd-notifications-config/
├── Chart.yaml
├── values.yaml
└── templates/
    ├── cm.yaml             # argocd-notifications-cm
    └── secret.yaml         # argocd-notifications-secret (현재 빈 secret)
```

**values.yaml 예시:**
```yaml
webhookUrl: http://webhook-receiver.argocd-noti-receiver.svc.cluster.local
targetNamespace: argocd-noti-test
argocdUrl: https://argocd-server.argocd.svc.cluster.local
```

### 5.2 `chart/webhook-receiver/` (신규 Helm chart)

HTTP webhook을 받아 payload를 stdout JSON으로 출력.

**구조:**
```
chart/webhook-receiver/
├── Chart.yaml
├── values.yaml
└── templates/
    ├── deployment.yaml     # mendhak/http-https-echo, replicas: 1
    └── service.yaml        # ClusterIP, port 80 → 8080
```

확인 방법: `kubectl logs -f deployment/webhook-receiver -n argocd-noti-receiver`

### 5.3 `bootstrap/notifications/` (신규)

위 2개 Helm chart를 ArgoCD가 sync하도록 Application 리소스 정의.

**구조:**
```
bootstrap/notifications/
├── app-argocd-notifications-config.yaml   # → chart/argocd-notifications-config sync
└── app-webhook-receiver.yaml              # → chart/webhook-receiver sync
```

**`app-argocd-notifications-config.yaml`:**
- destination namespace: `argocd`
- syncPolicy: `automated` (prune + selfHeal 모두 true) — 알림 설정 변경 즉시 반영
- syncOptions:
  - `ServerSideApply=true` — argo-cd Helm chart가 이미 만든 `argocd-notifications-cm` 및 `argocd-notifications-secret`의 ownership을 field-level merge로 인수

**`app-webhook-receiver.yaml`:**
- destination namespace: `argocd-noti-receiver`
- syncPolicy: `automated` (prune + selfHeal 모두 true)
- syncOptions:
  - `CreateNamespace=true` — `argocd-noti-receiver` namespace 자동 생성

### 5.4 `bootstrap/application-set/appset-noti-test.yaml` (신규)

`argocd-noti-test` namespace로 `hello-world-server`를 sync하는 ApplicationSet.

- Generator: **List Generator** (기존 `appset-list.yaml`과 동일 방식, element는 1개)
- destination namespace: `argocd-noti-test`
- syncPolicy: `automated` **제거** (manual sync 모드, selfHeal 없음)
- syncOptions: `CreateNamespace=true`

**기존 `appset-list.yaml`과의 차이:**

| 항목 | 기존 (`appset-list`) | 신규 (`appset-noti-test`) |
|---|---|---|
| destination namespace | `argocd-test` | `argocd-noti-test` |
| `automated` syncPolicy | 있음 (selfHeal: true) | 없음 (manual sync) |
| 대상 차트 | 3개 (echo, hello, hook) | 1개 (`hello-world-server`만) |
| CreateNamespace | true | true |

### 5.5 `chart/hello-world-server/` (기존, 변경 없음)

기존 차트를 그대로 재사용. ApplicationSet이 destination namespace만 다르게 sync.

## 6. 설치 순서 및 의존성

```bash
# 1) webhook-receiver 먼저 (notifications-controller가 처음 알림 발송 시 destination이 존재하도록)
kubectl apply -f bootstrap/notifications/app-webhook-receiver.yaml

# 2) notifications 설정
kubectl apply -f bootstrap/notifications/app-argocd-notifications-config.yaml

# 3) 알림 테스트 대상 ApplicationSet
kubectl apply -f bootstrap/application-set/appset-noti-test.yaml
```

webhook-receiver를 먼저 띄우는 이유: notifications-config 활성 시 webhook URL이 미리 존재하여 첫 알림 실패 방지.

## 7. 알림 발생 흐름 (trigger, template, payload)

### 7.1 Triggers (3개)

모든 trigger에 동일한 namespace 필터(`destination.namespace == 'argocd-noti-test'`)를 적용한다.

```yaml
# 1) OutOfSync (커스텀)
trigger.on-sync-status-out-of-sync: |
  - when: |
      app.spec.destination.namespace == 'argocd-noti-test' &&
      app.status.sync.status == 'OutOfSync'
    send: [app-out-of-sync]
    oncePer: app.status.sync.revision

# 2) Sync Failed (카탈로그 표준 기반 + namespace 필터)
trigger.on-sync-failed: |
  - when: |
      app.spec.destination.namespace == 'argocd-noti-test' &&
      app.status.operationState.phase in ['Error', 'Failed']
    send: [app-sync-failed]
    oncePer: app.status.operationState.startedAt

# 3) Health Degraded (카탈로그 표준 기반 + namespace 필터)
trigger.on-health-degraded: |
  - when: |
      app.spec.destination.namespace == 'argocd-noti-test' &&
      app.status.health.status == 'Degraded'
    send: [app-health-degraded]
```

`oncePer` 정책:
- OutOfSync: revision당 1회 (같은 drift 반복 알림 방지)
- Sync Failed: sync 시도(startedAt)당 1회 (같은 실패 반복 알림 방지)
- Health Degraded: 별도 oncePer 없음 — Degraded → Healthy → Degraded 전환 시마다 알림 (ArgoCD가 상태 전이 시점에 1회 발송)

### 7.2 Templates (3개 — trigger별 강조 정보 다름)

**공통 base 구조**: `event`, `timestamp`, `application`, `sync`, `health`, `source`, `argocdUrl`
**trigger별 추가**: 해당 이벤트의 핵심 정보

```yaml
# 1) OutOfSync — sync revision 강조
template.app-out-of-sync: |
  webhook:
    local-receiver:
      method: POST
      body: |
        {
          "event": "argocd.out-of-sync",
          "severity": "info",
          "timestamp": "{{ (call .time.Now).Format "2006-01-02T15:04:05Z07:00" }}",
          "application": {
            "name": "{{.app.metadata.name}}",
            "namespace": "{{.app.spec.destination.namespace}}",
            "project": "{{.app.spec.project}}"
          },
          "sync": {
            "status": "{{.app.status.sync.status}}",
            "revision": "{{.app.status.sync.revision}}"
          },
          "health": { "status": "{{.app.status.health.status}}" },
          "source": {
            "repoURL": "{{.app.spec.source.repoURL}}",
            "targetRevision": "{{.app.spec.source.targetRevision}}",
            "path": "{{.app.spec.source.path}}"
          },
          "argocdUrl": "{{.context.argocdUrl}}/applications/{{.app.metadata.name}}"
        }

# 2) Sync Failed — operationState 강조 (실패 메시지, sync 시도 시간)
template.app-sync-failed: |
  webhook:
    local-receiver:
      method: POST
      body: |
        {
          "event": "argocd.sync-failed",
          "severity": "error",
          "timestamp": "{{ (call .time.Now).Format "2006-01-02T15:04:05Z07:00" }}",
          "application": {
            "name": "{{.app.metadata.name}}",
            "namespace": "{{.app.spec.destination.namespace}}",
            "project": "{{.app.spec.project}}"
          },
          "operation": {
            "phase": "{{.app.status.operationState.phase}}",
            "message": "{{.app.status.operationState.message}}",
            "startedAt": "{{.app.status.operationState.startedAt}}",
            "finishedAt": "{{.app.status.operationState.finishedAt}}"
          },
          "sync": {
            "status": "{{.app.status.sync.status}}",
            "revision": "{{.app.status.sync.revision}}"
          },
          "argocdUrl": "{{.context.argocdUrl}}/applications/{{.app.metadata.name}}"
        }

# 3) Health Degraded — health 상세 강조 (어느 리소스가 비정상인지)
template.app-health-degraded: |
  webhook:
    local-receiver:
      method: POST
      body: |
        {
          "event": "argocd.health-degraded",
          "severity": "warning",
          "timestamp": "{{ (call .time.Now).Format "2006-01-02T15:04:05Z07:00" }}",
          "application": {
            "name": "{{.app.metadata.name}}",
            "namespace": "{{.app.spec.destination.namespace}}",
            "project": "{{.app.spec.project}}"
          },
          "health": {
            "status": "{{.app.status.health.status}}"
          },
          "resources": [
            {{- range $i, $r := .app.status.resources }}
            {{- if and $r.health (ne $r.health.status "Healthy") }}
            {{- if $i }},{{- end }}
            {
              "kind": "{{$r.kind}}",
              "name": "{{$r.name}}",
              "status": "{{$r.health.status}}",
              "message": "{{$r.health.message}}"
            }
            {{- end }}
            {{- end }}
          ],
          "sync": { "status": "{{.app.status.sync.status}}" },
          "argocdUrl": "{{.context.argocdUrl}}/applications/{{.app.metadata.name}}"
        }
```

### 7.3 Service

```yaml
service.webhook.local-receiver: |
  url: http://webhook-receiver.argocd-noti-receiver.svc.cluster.local
  headers:
  - name: Content-Type
    value: application/json
```

### 7.4 Default Subscription (3개 trigger 모두)

```yaml
subscriptions: |
  - recipients:
    - webhook:local-receiver
    triggers:
    - on-sync-status-out-of-sync
    - on-sync-failed
    - on-health-degraded
```

모든 Application이 subscription을 가지지만 각 trigger의 `when` 조건이 namespace 필터링을 담당.

### 7.5 Timing 특성

| 단계 | 기본 지연 |
|---|---|
| Cluster drift 감지 (application-controller) | ~10초 |
| Git polling (repo refresh) | 3분 (또는 `argocd app get <name> --refresh`로 즉시) |
| Notifications polling | 60초 |
| 같은 revision 중복 알림 | `oncePer`로 방지 |

Cluster drift는 보통 10~70초 내 알림 수신, Git drift는 manual refresh 사용 시 즉시 가능.

## 8. 테스트 시나리오

### 8.1 정상 설치 검증

```bash
kubectl get application -n argocd
# webhook-receiver           Synced  Healthy
# argocd-notifications-config Synced  Healthy
# hello-world-server         Synced  Healthy

kubectl get pod -n argocd-noti-receiver       # running
kubectl get pod -n argocd-noti-test           # running
kubectl get cm argocd-notifications-cm -n argocd -o yaml | grep on-sync-status-out-of-sync
```

### 8.2 Cluster Drift

```bash
# 로그 tail
kubectl logs -f deployment/webhook-receiver -n argocd-noti-receiver

# drift 발생
kubectl scale deployment hello-world-server -n argocd-noti-test --replicas=3
```

**기대:**
- ~10초 내 Application status가 OutOfSync
- selfHeal 없음 → 자동 sync 발생 안 함
- ~60초 내 webhook-receiver 로그에 OutOfSync payload 출력
- payload의 `sync.status == "OutOfSync"`, `application.name == "hello-world-server"` 확인

**복구:**
```bash
argocd app sync hello-world-server
```

### 8.3 Git Drift

```bash
# values.yaml 변경 후 push
git commit -am "test: trigger out-of-sync"
git push

argocd app get hello-world-server --refresh   # 즉시 감지
```

**기대:** ~60초 내 알림 도착 (revision은 새 커밋 hash).

### 8.4 Sync Failed (manual sync 실패)

`hello-world-server`의 helm values에 잘못된 필드를 넣어 sync 시도를 실패시킨다.

```bash
# values.yaml에 의도적으로 잘못된 image 필드 추가 (예: 존재하지 않는 image registry)
# 또는 잘못된 helm template 문법 추가
git commit -am "test: trigger sync failure"
git push

argocd app sync hello-world-server   # manual sync 시도 → 실패
```

**기대:**
- `kubectl get application hello-world-server -n argocd`에서 `operationState.phase: Failed/Error`
- ~60초 내 webhook-receiver 로그에 `event: argocd.sync-failed` payload 출력
- payload의 `operation.message`에 실패 원인 포함

**복구:** git revert 또는 values.yaml 원복 후 push, manual sync.

### 8.5 Health Degraded (런타임 비정상)

배포된 Pod을 비정상화시켜 health-degraded 알림을 유발한다.

```bash
# 방법 A: image tag를 존재하지 않는 값으로 변경 → ImagePullBackOff
git에서 chart/hello-world-server/values.yaml의 image tag를 존재하지 않는 값으로 변경 후 push
argocd app sync hello-world-server

# 방법 B: deployment를 직접 patch (drift도 함께 발생)
kubectl set image deployment/hello-world-server hello-world-server=nonexistent:image -n argocd-noti-test
```

**기대:**
- ~30초 내 Application status `health.status: Degraded`
- ~60초 내 webhook-receiver 로그에 `event: argocd.health-degraded` payload 출력
- payload의 `resources` 배열에 비정상 리소스 정보 (kind, name, status: Degraded/Missing, message)

**복구:** image tag 원복 후 sync.

### 8.6 다른 namespace는 알림 안 옴 (negative test)

```bash
# 기존 argocd-test의 echo-server에 drift 유발
kubectl scale deployment echo-server -n argocd-test --replicas=3
```

**기대:** 모든 trigger의 `when` 조건에 namespace 필터가 있으므로 webhook 호출 없음. webhook-receiver 로그에 새 출력 없음.

### 8.7 중복 알림 방지

- OutOfSync: 같은 revision의 OutOfSync는 1회만 발송
- Sync Failed: 같은 sync 시도(startedAt)에 대해 1회만 발송. 재시도 시 새 알림
- Health Degraded: ArgoCD가 상태 전이 시점(Healthy → Degraded)에 발송. Degraded → Healthy → Degraded 사이클 시 발송 반복

## 9. 에러 처리 및 운영 고려사항

| 상황 | 처리 |
|---|---|
| webhook-receiver 다운 | notifications-controller가 error 로그만 남기고 retry 없음. `oncePer`로 같은 revision은 재발송 X → 새 drift 유발해 재검증 필요 |
| trigger 표현식 syntax 에러 | notifications-controller CrashLoopBackOff → `kubectl logs -n argocd deployment/argocd-notifications-controller`로 확인. CM 수정 후 다음 ArgoCD sync에서 복구 |
| CM ownership 충돌 | `ServerSideApply=true`로 field-level merge — Helm은 자체 키만, ArgoCD Application은 우리 trigger/template/service 키만 관리 |
| Controller가 새 CM을 reload? | controller가 ConfigMap을 watch → 자동 reload (Pod 재시작 불필요) |
| Default subscription이 다른 ns 앱에도 영향? | 모든 app이 subscription을 갖지만 trigger의 `when` 조건이 false면 발송 안 됨 (시나리오 8.4로 검증) |

## 10. Rollback / Cleanup

```bash
kubectl delete -f bootstrap/notifications/
kubectl delete -f bootstrap/application-set/appset-noti-test.yaml

# 자동으로 정리됨:
#   - Application 3개 삭제
#   - argocd-notifications-cm은 빈 상태로 복귀 (Helm chart의 빈 키만 남음)
#   - argocd-noti-receiver, argocd-noti-test namespace 삭제
#
# ArgoCD 자체는 영향 없음 (helm_release 무관)
```

## 11. 의사결정 트레일

| 결정 | 이유 |
|---|---|
| Helm chart로 알림 설정 분리 (Terraform values에 추가 X) | production에서 ArgoCD 재배포 어려움 → 알림 설정만 GitOps로 분리 |
| Default subscription + trigger 내 namespace 필터 | 운영팀 중앙 관리 패턴. ApplicationSet마다 annotation 부여하지 않아도 됨 |
| webhook-receiver를 별도 namespace로 격리 | 자기 자신이 알림 대상이 되는 것 방지 + 책임 분리 |
| 3개 trigger 함께 (OutOfSync + Sync Failed + Health Degraded) | OutOfSync로 잡지 못하는 영역(런타임 헬스 저하, manual sync 실패) 커버. 운영 시나리오에 가까움 |
| trigger마다 별도 template (강조 정보 다름) | 학습 측면: ArgoCD template 시스템 깊이 이해. payload 분석 시 어떤 정보가 의미 있는지 명확 |
| `oncePer` 정책 trigger별 다르게 | OutOfSync는 revision, Sync Failed는 startedAt, Health Degraded는 상태 전이 (ArgoCD 기본) |
| `automated` 완전 제거 (sync도, selfHeal도) | Git drift + Cluster drift + manual sync 실패 시나리오 모두 의미 있게 발생 |
| 테스트 대상 1개 차트로 단순화 (`hello-world-server`만) | 학습/검증 목적엔 1개로 충분, 시나리오 재현이 쉬움 |

## 12. 범위 외 (Out of Scope)

- Slack, Email 등 다른 notification service 통합 (향후 별도 task)
- 추가 trigger (`on-deployed`, `on-sync-succeeded`, `on-created`, `on-deleted` 등) — 학습/운영 가치 낮거나 노이즈 가능
- AppProject 단위 subscription
- 다중 환경(dev/staging/prod) values 분리 (단일 클러스터 단일 환경)
- argocd-notifications-controller의 metrics/observability 설정
- 알림 발송 실패 시의 retry/DLQ 설계
