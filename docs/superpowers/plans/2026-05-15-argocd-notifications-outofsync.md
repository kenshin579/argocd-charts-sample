# ArgoCD Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** ArgoCD Notifications를 통해 `argocd-noti-test` namespace의 모든 Application에서 발생하는 OutOfSync / Sync Failed / Health Degraded 이벤트를 클러스터 내부 webhook receiver로 알림 받는 로컬 테스트 환경을 구축한다.

**Architecture:** 기존 ArgoCD Helm release는 건드리지 않고, 알림 설정을 별도 Helm chart(`argocd-notifications-config`)로 분리한다. ArgoCD Application 리소스가 자기 자신의 알림 설정을 sync하면서 기존 ConfigMap/Secret의 ownership을 `ServerSideApply`로 인수한다. webhook receiver는 별도 namespace에 격리.

**Tech Stack:** ArgoCD 7.8.28 (Helm chart), Kubernetes (Docker Desktop), Helm 3, `mendhak/http-https-echo` (webhook receiver), bash + kubectl.

**Spec:** `docs/superpowers/specs/2026-05-15-argocd-notifications-outofsync-design.md`

**Pre-condition:**
- ArgoCD가 이미 설치되어 있어야 함 (`cd terraform && make tf-infra`로 사전 설치)
- 현재 브랜치: `feat/argocd-notifications-outofsync`
- kubectl context: `docker-desktop`

---

## File Structure

```
chart/argocd-notifications-config/          (신규 chart - 알림 설정)
├── Chart.yaml
├── values.yaml
└── templates/
    ├── cm.yaml                              (argocd-notifications-cm: triggers + templates + service + subscription)
    └── secret.yaml                          (argocd-notifications-secret: 빈 secret, ownership 인수용)

chart/webhook-receiver/                      (신규 chart - 수신 서버)
├── Chart.yaml
├── values.yaml
└── templates/
    ├── deployment.yaml                      (mendhak/http-https-echo)
    └── service.yaml                         (ClusterIP, port 80 → 8080)

bootstrap/notifications.yaml                 (신규 - multi-doc Application 2개)
bootstrap/application-set/appset-noti-test.yaml  (신규 - List Generator, hello-world-server 1개)

README.md                                    (수정 - 알림 섹션 추가)
```

각 파일의 책임:
- `chart/argocd-notifications-config/templates/cm.yaml` — ArgoCD Notifications의 모든 동작 정의 (trigger 3개, template 3개, service 1개, subscription)
- `chart/argocd-notifications-config/templates/secret.yaml` — `argocd-notifications-secret` 의 ownership 인수용 빈 secret (현재 외부 자격증명 없음, future-proof)
- `chart/webhook-receiver/templates/*` — webhook 수신 Deployment + Service
- `bootstrap/notifications.yaml` — 두 chart를 sync하는 Application 정의 (multi-doc YAML)
- `bootstrap/application-set/appset-noti-test.yaml` — 알림 테스트 대상 앱(`hello-world-server`)을 `argocd-noti-test` namespace로 배포 (`automated` 없음)

---

## Task 1: `chart/argocd-notifications-config` Helm chart 작성

**Files:**
- Create: `chart/argocd-notifications-config/Chart.yaml`
- Create: `chart/argocd-notifications-config/values.yaml`
- Create: `chart/argocd-notifications-config/templates/cm.yaml`
- Create: `chart/argocd-notifications-config/templates/secret.yaml`

- [ ] **Step 1.1: Chart.yaml 생성**

`chart/argocd-notifications-config/Chart.yaml`:

```yaml
apiVersion: v2
name: argocd-notifications-config
description: ArgoCD Notifications configuration (triggers, templates, services, subscriptions) for argocd-noti-test
type: application
version: 0.1.0
appVersion: "1.0.0"
```

- [ ] **Step 1.2: values.yaml 생성**

`chart/argocd-notifications-config/values.yaml`:

```yaml
# webhook receiver의 cluster-internal URL
webhookUrl: http://webhook-receiver.argocd-noti-receiver.svc.cluster.local

# 알림 trigger에 적용할 destination namespace 필터
targetNamespace: argocd-noti-test

# template body에 포함되는 ArgoCD UI base URL
argocdUrl: https://argocd-server.argocd.svc.cluster.local
```

- [ ] **Step 1.3: templates/cm.yaml 생성 (trigger 3개 + template 3개 + service + subscription)**

`chart/argocd-notifications-config/templates/cm.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-notifications-cm
  namespace: argocd
data:
  # ======= Triggers =======
  trigger.on-sync-status-out-of-sync: |
    - when: |
        app.spec.destination.namespace == '{{ .Values.targetNamespace }}' &&
        app.status.sync.status == 'OutOfSync'
      send: [app-out-of-sync]
      oncePer: app.status.sync.revision

  trigger.on-sync-failed: |
    - when: |
        app.spec.destination.namespace == '{{ .Values.targetNamespace }}' &&
        app.status.operationState.phase in ['Error', 'Failed']
      send: [app-sync-failed]
      oncePer: app.status.operationState.startedAt

  trigger.on-health-degraded: |
    - when: |
        app.spec.destination.namespace == '{{ .Values.targetNamespace }}' &&
        app.status.health.status == 'Degraded'
      send: [app-health-degraded]

  # ======= Templates =======
  template.app-out-of-sync: |
    webhook:
      local-receiver:
        method: POST
        body: |
          {
            "event": "argocd.out-of-sync",
            "severity": "info",
            "timestamp": "{{`{{ (call .time.Now).Format "2006-01-02T15:04:05Z07:00" }}`}}",
            "application": {
              "name": "{{`{{.app.metadata.name}}`}}",
              "namespace": "{{`{{.app.spec.destination.namespace}}`}}",
              "project": "{{`{{.app.spec.project}}`}}"
            },
            "sync": {
              "status": "{{`{{.app.status.sync.status}}`}}",
              "revision": "{{`{{.app.status.sync.revision}}`}}"
            },
            "health": { "status": "{{`{{.app.status.health.status}}`}}" },
            "source": {
              "repoURL": "{{`{{.app.spec.source.repoURL}}`}}",
              "targetRevision": "{{`{{.app.spec.source.targetRevision}}`}}",
              "path": "{{`{{.app.spec.source.path}}`}}"
            },
            "argocdUrl": "{{ .Values.argocdUrl }}/applications/{{`{{.app.metadata.name}}`}}"
          }

  template.app-sync-failed: |
    webhook:
      local-receiver:
        method: POST
        body: |
          {
            "event": "argocd.sync-failed",
            "severity": "error",
            "timestamp": "{{`{{ (call .time.Now).Format "2006-01-02T15:04:05Z07:00" }}`}}",
            "application": {
              "name": "{{`{{.app.metadata.name}}`}}",
              "namespace": "{{`{{.app.spec.destination.namespace}}`}}",
              "project": "{{`{{.app.spec.project}}`}}"
            },
            "operation": {
              "phase": "{{`{{.app.status.operationState.phase}}`}}",
              "message": "{{`{{.app.status.operationState.message}}`}}",
              "startedAt": "{{`{{.app.status.operationState.startedAt}}`}}",
              "finishedAt": "{{`{{.app.status.operationState.finishedAt}}`}}"
            },
            "sync": {
              "status": "{{`{{.app.status.sync.status}}`}}",
              "revision": "{{`{{.app.status.sync.revision}}`}}"
            },
            "argocdUrl": "{{ .Values.argocdUrl }}/applications/{{`{{.app.metadata.name}}`}}"
          }

  template.app-health-degraded: |
    webhook:
      local-receiver:
        method: POST
        body: |
          {
            "event": "argocd.health-degraded",
            "severity": "warning",
            "timestamp": "{{`{{ (call .time.Now).Format "2006-01-02T15:04:05Z07:00" }}`}}",
            "application": {
              "name": "{{`{{.app.metadata.name}}`}}",
              "namespace": "{{`{{.app.spec.destination.namespace}}`}}",
              "project": "{{`{{.app.spec.project}}`}}"
            },
            "health": {
              "status": "{{`{{.app.status.health.status}}`}}"
            },
            "resources": [
              {{`{{- range $i, $r := .app.status.resources }}`}}
              {{`{{- if and $r.health (ne $r.health.status "Healthy") }}`}}
              {{`{{- if $i }},{{- end }}`}}
              {
                "kind": "{{`{{$r.kind}}`}}",
                "name": "{{`{{$r.name}}`}}",
                "status": "{{`{{$r.health.status}}`}}",
                "message": "{{`{{$r.health.message}}`}}"
              }
              {{`{{- end }}`}}
              {{`{{- end }}`}}
            ],
            "sync": { "status": "{{`{{.app.status.sync.status}}`}}" },
            "argocdUrl": "{{ .Values.argocdUrl }}/applications/{{`{{.app.metadata.name}}`}}"
          }

  # ======= Service =======
  service.webhook.local-receiver: |
    url: {{ .Values.webhookUrl }}
    headers:
    - name: Content-Type
      value: application/json

  # ======= Default Subscription =======
  subscriptions: |
    - recipients:
      - webhook:local-receiver
      triggers:
      - on-sync-status-out-of-sync
      - on-sync-failed
      - on-health-degraded
```

**중요**: 이 chart에서 `{{`...`}}` 패턴이 자주 나오는 이유 — Helm template 안에 ArgoCD Notifications template(`{{.app.metadata.name}}` 등)이 그대로 들어가야 한다. Helm은 `{{` 그대로를 출력하려면 `{{`...`}}`로 escape 필요. **`webhookUrl`, `targetNamespace`, `argocdUrl` 3개 변수만 Helm이 치환**하고 나머지 `{{ ... }}`는 ArgoCD Notifications가 런타임에 처리.

- [ ] **Step 1.4: templates/secret.yaml 생성**

`chart/argocd-notifications-config/templates/secret.yaml`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: argocd-notifications-secret
  namespace: argocd
type: Opaque
data: {}
```

빈 secret. argo-cd Helm chart가 이미 같은 이름의 secret을 만들기 때문에 `ServerSideApply`로 ownership 인수만 한다. 향후 Slack token 등 추가될 때 이 secret에 키 추가.

- [ ] **Step 1.5: helm lint로 chart 문법 검증**

Run: `helm lint chart/argocd-notifications-config`
Expected: `1 chart(s) linted, 0 chart(s) failed`

- [ ] **Step 1.6: helm template으로 렌더링 결과 검증**

Run: `helm template test chart/argocd-notifications-config | grep -E "on-sync-status-out-of-sync|on-sync-failed|on-health-degraded|local-receiver"`
Expected: 위 4개 키워드가 모두 출력되어야 함 (4줄 이상).

- [ ] **Step 1.7: helm template으로 namespace 필터 치환 검증**

Run: `helm template test chart/argocd-notifications-config | grep "argocd-noti-test"`
Expected: 최소 3개 (3개 trigger 각각에서 namespace 필터로 사용됨).

- [ ] **Step 1.8: kubectl apply --dry-run으로 manifest 유효성 검증**

Run: `helm template test chart/argocd-notifications-config | kubectl apply --dry-run=client -f -`
Expected: `configmap/argocd-notifications-cm created (dry run)` + `secret/argocd-notifications-secret created (dry run)`. 에러 없음.

- [ ] **Step 1.9: Commit**

```bash
git add chart/argocd-notifications-config/
git commit -m "[feat/argocd-notifications-outofsync] argocd-notifications-config Helm chart 추가

* triggers: on-sync-status-out-of-sync, on-sync-failed, on-health-degraded
* templates: app-out-of-sync, app-sync-failed, app-health-degraded
* service: webhook local-receiver
* default subscription: 3개 trigger 모두 구독"
```

---

## Task 2: `chart/webhook-receiver` Helm chart 작성

**Files:**
- Create: `chart/webhook-receiver/Chart.yaml`
- Create: `chart/webhook-receiver/values.yaml`
- Create: `chart/webhook-receiver/templates/deployment.yaml`
- Create: `chart/webhook-receiver/templates/service.yaml`

- [ ] **Step 2.1: Chart.yaml 생성**

`chart/webhook-receiver/Chart.yaml`:

```yaml
apiVersion: v2
name: webhook-receiver
description: HTTP webhook receiver that logs incoming requests to stdout (mendhak/http-https-echo)
type: application
version: 0.1.0
appVersion: "37"
```

- [ ] **Step 2.2: values.yaml 생성**

`chart/webhook-receiver/values.yaml`:

```yaml
image:
  repository: mendhak/http-https-echo
  tag: "37"

replicaCount: 1

service:
  port: 80
  targetPort: 8080
```

- [ ] **Step 2.3: templates/deployment.yaml 생성**

`chart/webhook-receiver/templates/deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: webhook-receiver
spec:
  replicas: {{ .Values.replicaCount | default 1 }}
  selector:
    matchLabels:
      app: webhook-receiver
  template:
    metadata:
      labels:
        app: webhook-receiver
    spec:
      containers:
        - name: webhook-receiver
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          env:
            - name: HTTP_PORT
              value: "8080"
            - name: LOG_WITHOUT_NEWLINE
              value: "false"
          ports:
            - containerPort: 8080
```

`mendhak/http-https-echo` 환경변수:
- `HTTP_PORT=8080` — 컨테이너가 listen할 포트
- `LOG_WITHOUT_NEWLINE=false` — JSON payload 사이에 newline 표시 (가독성)

- [ ] **Step 2.4: templates/service.yaml 생성**

`chart/webhook-receiver/templates/service.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: webhook-receiver
spec:
  type: ClusterIP
  selector:
    app: webhook-receiver
  ports:
    - protocol: TCP
      port: {{ .Values.service.port }}
      targetPort: {{ .Values.service.targetPort }}
```

Service FQDN: `webhook-receiver.argocd-noti-receiver.svc.cluster.local:80`. ArgoCD notifications-controller가 이 URL로 POST.

- [ ] **Step 2.5: helm lint로 검증**

Run: `helm lint chart/webhook-receiver`
Expected: `1 chart(s) linted, 0 chart(s) failed`

- [ ] **Step 2.6: helm template으로 검증**

Run: `helm template test chart/webhook-receiver | grep -E "mendhak/http-https-echo|webhook-receiver"`
Expected: image 라인 + name/label 라인들 출력.

- [ ] **Step 2.7: kubectl --dry-run으로 manifest 유효성 검증**

Run: `helm template test chart/webhook-receiver | kubectl apply --dry-run=client -f -`
Expected: `deployment.apps/webhook-receiver created (dry run)` + `service/webhook-receiver created (dry run)`.

- [ ] **Step 2.8: Commit**

```bash
git add chart/webhook-receiver/
git commit -m "[feat/argocd-notifications-outofsync] webhook-receiver Helm chart 추가

* mendhak/http-https-echo:37 이미지 사용
* Deployment + ClusterIP Service (port 80 → 8080)
* 받은 HTTP 요청을 stdout JSON으로 출력"
```

---

## Task 3: `bootstrap/notifications.yaml` 작성 (multi-doc Application 2개)

**Files:**
- Create: `bootstrap/notifications.yaml`

- [ ] **Step 3.1: bootstrap/notifications.yaml 생성 (multi-doc YAML)**

`bootstrap/notifications.yaml`:

```yaml
# 알림 시스템 부트스트랩 — 2개의 Application을 한 파일에 정의
# 1) webhook-receiver: HTTP webhook을 받아 stdout으로 출력 (argocd-noti-receiver ns)
# 2) argocd-notifications-config: trigger/template/service/subscription 설정 (argocd ns)
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: webhook-receiver
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/kenshin579/argocd-charts-sample
    targetRevision: HEAD
    path: chart/webhook-receiver
    helm:
      valueFiles:
        - values.yaml
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd-noti-receiver
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: argocd-notifications-config
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/kenshin579/argocd-charts-sample
    targetRevision: HEAD
    path: chart/argocd-notifications-config
    helm:
      valueFiles:
        - values.yaml
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      # argo-cd Helm chart가 이미 만든 argocd-notifications-cm/secret의 ownership을
      # field-level merge로 인수
      - ServerSideApply=true
```

- [ ] **Step 3.2: kubectl --dry-run으로 manifest 유효성 검증**

Run: `kubectl apply --dry-run=client -f bootstrap/notifications.yaml`
Expected:
```
application.argoproj.io/webhook-receiver created (dry run)
application.argoproj.io/argocd-notifications-config created (dry run)
```

- [ ] **Step 3.3: multi-doc YAML 구분자(`---`) 확인**

Run: `grep -c "^---$" bootstrap/notifications.yaml`
Expected: `2` (첫 Application 앞과 두 Application 사이).

- [ ] **Step 3.4: Commit**

```bash
git add bootstrap/notifications.yaml
git commit -m "[feat/argocd-notifications-outofsync] bootstrap/notifications.yaml 추가

* multi-doc YAML로 두 Application을 한 파일에
* webhook-receiver: argocd-noti-receiver ns로 sync (CreateNamespace=true)
* argocd-notifications-config: argocd ns로 sync (ServerSideApply=true)"
```

---

## Task 4: `bootstrap/application-set/appset-noti-test.yaml` 작성

**Files:**
- Create: `bootstrap/application-set/appset-noti-test.yaml`

- [ ] **Step 4.1: ApplicationSet 작성 (List Generator, hello-world-server 1개, automated 없음)**

`bootstrap/application-set/appset-noti-test.yaml`:

```yaml
# 알림 테스트 대상 — argocd-noti-test namespace로 hello-world-server를 sync.
# 기존 appset-list.yaml과의 차이:
#   - destination namespace: argocd-noti-test (격리)
#   - syncPolicy.automated 제거 (manual sync, selfHeal 없음)
#   - 대상 차트 1개로 단순화
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: noti-test-applications
  namespace: argocd
spec:
  generators:
    - list:
        elements:
          - name: hello-world-server
            path: chart/hello-world-server
            namespace: argocd-noti-test
  template:
    metadata:
      name: '{{name}}'
    spec:
      project: default
      source:
        repoURL: https://github.com/kenshin579/argocd-charts-sample
        targetRevision: HEAD
        path: '{{path}}'
        helm:
          valueFiles:
            - values.yaml
      destination:
        server: https://kubernetes.default.svc
        namespace: '{{namespace}}'
      # syncPolicy.automated 없음 — manual sync (Git/Cluster drift 모두 의미 있게 발생)
      syncPolicy:
        syncOptions:
          - CreateNamespace=true
```

- [ ] **Step 4.2: kubectl --dry-run으로 검증**

Run: `kubectl apply --dry-run=client -f bootstrap/application-set/appset-noti-test.yaml`
Expected: `applicationset.argoproj.io/noti-test-applications created (dry run)`.

- [ ] **Step 4.3: automated 미존재 확인**

Run: `grep -A 3 "syncPolicy" bootstrap/application-set/appset-noti-test.yaml`
Expected: `syncPolicy` 아래에 `automated` 키가 없고 `syncOptions`만 있어야 함.

- [ ] **Step 4.4: Commit**

```bash
git add bootstrap/application-set/appset-noti-test.yaml
git commit -m "[feat/argocd-notifications-outofsync] appset-noti-test ApplicationSet 추가

* List Generator로 hello-world-server를 argocd-noti-test ns로 배포
* syncPolicy.automated 제거 — manual sync 모드 (Git/Cluster drift 검증용)
* CreateNamespace=true"
```

---

## Task 5: 클러스터 배포 + 정상 설치 검증 (시나리오 8.1)

이 task는 git에 push가 필요 — ArgoCD가 git에서 chart를 sync하기 때문. 로컬에서만 검증하려면 별도 git remote 또는 ArgoCD의 path 변경이 필요한데, 이 plan에서는 **변경 사항을 origin에 push**한다고 가정한다.

**Pre-condition:**
- ArgoCD가 이미 설치되어 있음 (`kubectl get pods -n argocd` 결과에 controller/server/repo-server 등이 Running).
- 위 4개 task의 변경 사항이 git origin/main 또는 작업 브랜치에 push되어 있어 ArgoCD가 fetch 가능 (또는 `targetRevision`을 작업 브랜치로 변경).

- [ ] **Step 5.1: 작업 브랜치를 origin에 push**

Run:
```bash
git push -u origin feat/argocd-notifications-outofsync
```
Expected: push 성공.

- [ ] **Step 5.2: bootstrap/notifications.yaml 및 appset의 targetRevision 변경 (테스트 동안만)**

bootstrap의 두 yaml에서 `targetRevision: HEAD` 를 `targetRevision: feat/argocd-notifications-outofsync` 로 임시 변경. (PR merge 후 다시 `HEAD`로 되돌릴 예정. 이 임시 변경은 commit 하지 않는다.)

Run:
```bash
sed -i.bak 's|targetRevision: HEAD|targetRevision: feat/argocd-notifications-outofsync|g' \
  bootstrap/notifications.yaml \
  bootstrap/application-set/appset-noti-test.yaml
```

Run: `grep -n targetRevision bootstrap/notifications.yaml bootstrap/application-set/appset-noti-test.yaml`
Expected: 3개 라인 모두 `targetRevision: feat/argocd-notifications-outofsync`.

- [ ] **Step 5.3: 알림 시스템 부트스트랩 (Application 2개 동시 적용)**

Run:
```bash
kubectl apply -f bootstrap/notifications.yaml
```
Expected:
```
application.argoproj.io/webhook-receiver created
application.argoproj.io/argocd-notifications-config created
```

- [ ] **Step 5.4: Application sync 상태 대기 + 확인**

Run:
```bash
# 60초까지 대기
for i in $(seq 1 30); do
  status=$(kubectl get application webhook-receiver argocd-notifications-config -n argocd -o jsonpath='{range .items[*]}{.metadata.name}={.status.sync.status}/{.status.health.status} {end}')
  echo "[$i] $status"
  echo "$status" | grep -qv "Unknown" && echo "$status" | grep -q "Synced/Healthy.*Synced/Healthy" && break
  sleep 2
done

kubectl get application -n argocd
```

Expected: 두 Application 모두 `Synced / Healthy`.

- [ ] **Step 5.5: webhook-receiver Pod 정상 기동 확인**

Run:
```bash
kubectl get pod -n argocd-noti-receiver -l app=webhook-receiver
```
Expected: 1개 Pod `Running` 상태.

- [ ] **Step 5.6: argocd-notifications-cm에 우리 trigger가 들어가 있는지 확인 (ownership 인수 성공)**

Run:
```bash
kubectl get cm argocd-notifications-cm -n argocd -o yaml | grep -E "on-sync-status-out-of-sync|on-sync-failed|on-health-degraded"
```
Expected: 3개 trigger 키가 모두 출력.

- [ ] **Step 5.7: argocd-notifications-controller가 새 설정을 reload했는지 로그 확인**

Run:
```bash
kubectl logs -n argocd deployment/argocd-notifications-controller --tail=50 | grep -iE "reload|trigger"
```
Expected: 새 trigger를 인식했다는 로그 라인 (`Loaded trigger` 또는 `Configuration updated` 류). 없으면 controller pod 재시작: `kubectl rollout restart deployment/argocd-notifications-controller -n argocd`.

- [ ] **Step 5.8: ApplicationSet 적용 (테스트 대상 hello-world-server 배포)**

Run:
```bash
kubectl apply -f bootstrap/application-set/appset-noti-test.yaml
```
Expected: `applicationset.argoproj.io/noti-test-applications created`.

- [ ] **Step 5.9: hello-world-server Application 상태 확인**

Run:
```bash
for i in $(seq 1 30); do
  status=$(kubectl get application hello-world-server -n argocd -o jsonpath='{.status.sync.status}/{.status.health.status}' 2>/dev/null)
  echo "[$i] $status"
  [ "$status" = "Synced/Healthy" ] && break
  sleep 2
done
```

Expected: 최종 `Synced/Healthy` — automated 없으므로 자동 sync는 안 되지만, ApplicationSet이 생성한 Application은 초기 1회 sync가 발생 (또는 manual sync 필요: `argocd app sync hello-world-server`).

**Note**: `automated`가 없으면 첫 sync도 manual로 해야 할 수 있다. 그 경우:
```bash
argocd app sync hello-world-server  # 또는 UI에서 Sync 클릭
```
ArgoCD CLI 로그인이 안 되어 있다면 UI에서 sync.

- [ ] **Step 5.10: hello-world-server Pod 기동 확인**

Run:
```bash
kubectl get pod -n argocd-noti-test
```
Expected: `hello-world-server-xxx Running 1/1`.

- [ ] **Step 5.11: 정상 설치 완료 (시나리오 8.1 통과)**

Run:
```bash
kubectl get application -n argocd
kubectl get pod -n argocd-noti-receiver
kubectl get pod -n argocd-noti-test
```
Expected: 모두 정상.

이 task는 클러스터 상태 검증이므로 commit 불필요.

---

## Task 6: Cluster Drift 알림 검증 (시나리오 8.2)

- [ ] **Step 6.1: 별도 터미널에서 webhook-receiver 로그 tail 시작**

```bash
kubectl logs -f deployment/webhook-receiver -n argocd-noti-receiver
```
별도 터미널에서 계속 실행 유지. 새 알림이 도착하면 JSON payload가 출력된다.

- [ ] **Step 6.2: Cluster drift 유발 — replicas 변경**

Run:
```bash
kubectl scale deployment hello-world-server -n argocd-noti-test --replicas=3
```
Expected: `deployment.apps/hello-world-server scaled`.

- [ ] **Step 6.3: ~10초 대기 후 OutOfSync 감지 확인**

Run:
```bash
sleep 15
kubectl get application hello-world-server -n argocd -o jsonpath='{.status.sync.status}'
echo
```
Expected: `OutOfSync` 출력.

- [ ] **Step 6.4: selfHeal 없음 → 자동 sync 발생하지 않음 확인**

Run:
```bash
kubectl get deployment hello-world-server -n argocd-noti-test -o jsonpath='{.spec.replicas}'
echo
```
Expected: `3` (자동 복구되지 않고 유지).

- [ ] **Step 6.5: ~60초 내에 webhook-receiver 로그에 알림 도착 확인**

Step 6.1의 터미널에서 다음과 같은 JSON이 출력되어야 함 (대략 60초 내):

```json
{
  "method": "POST",
  "path": "/",
  ...
  "body": {
    "event": "argocd.out-of-sync",
    "severity": "info",
    "application": {
      "name": "hello-world-server",
      "namespace": "argocd-noti-test",
      ...
    },
    "sync": { "status": "OutOfSync", "revision": "..." },
    ...
  }
}
```

검증:
- `event: argocd.out-of-sync` 출현
- `application.name: hello-world-server`
- `application.namespace: argocd-noti-test`
- `sync.status: OutOfSync`

알림이 60초 내 도착하지 않으면:
```bash
kubectl logs -n argocd deployment/argocd-notifications-controller --tail=100 | grep -iE "error|hello-world-server"
```
로 notifications-controller 로그 확인.

- [ ] **Step 6.6: 복구 — manual sync로 drift 되돌리기**

Run:
```bash
argocd app sync hello-world-server
# 또는 ArgoCD UI에서 Sync 버튼
```

검증:
```bash
sleep 10
kubectl get deployment hello-world-server -n argocd-noti-test -o jsonpath='{.spec.replicas}'; echo
kubectl get application hello-world-server -n argocd -o jsonpath='{.status.sync.status}'; echo
```
Expected: `1`, `Synced`.

이 task는 검증만이고 commit 불필요.

---

## Task 7: Git Drift 알림 검증 (시나리오 8.3)

- [ ] **Step 7.1: webhook-receiver 로그 tail 유지 (Task 6과 동일)**

이미 Task 6에서 실행 중인 로그 tail을 유지.

- [ ] **Step 7.2: chart/hello-world-server/values.yaml의 replicaCount 변경**

`chart/hello-world-server/values.yaml`을 다음과 같이 수정 (replicaCount 키가 없으면 새로 추가):

```yaml
image:
  name_tag: kenshin579/hello-world-server:v0.4
replicaCount: 2
```

- [ ] **Step 7.3: commit + push (Git drift 발생)**

```bash
git add chart/hello-world-server/values.yaml
git commit -m "test: bump replicaCount to trigger Git drift"
git push
```

- [ ] **Step 7.4: ArgoCD refresh 트리거 — 즉시 git 변경 감지**

Run:
```bash
argocd app get hello-world-server --refresh
# 또는 UI에서 Refresh 버튼
```

- [ ] **Step 7.5: OutOfSync 상태 확인 (새 revision)**

Run:
```bash
sleep 10
kubectl get application hello-world-server -n argocd -o jsonpath='{.status.sync.status},{.status.sync.revision}'
echo
```
Expected: `OutOfSync,<새 커밋 hash>` — revision이 Task 6의 hash와 달라야 함.

- [ ] **Step 7.6: ~60초 내 webhook-receiver 로그에 알림 도착**

Step 6.1 터미널에서 새 JSON 출력 확인. 검증 포인트:
- `event: argocd.out-of-sync`
- `sync.revision`이 Step 7.3에서 푸시한 새 커밋 hash
- `oncePer`에 의해 같은 revision 중복 알림 없음

- [ ] **Step 7.7: 복구 (manual sync 후 git revert)**

```bash
argocd app sync hello-world-server   # 클러스터를 새 git 상태에 sync
# 그리고 테스트 후 revert
git revert HEAD --no-edit
git push
argocd app sync hello-world-server
```

이 task도 검증만, 별도 commit 불필요 (test commit은 revert로 되돌림).

---

## Task 8: Sync Failed 알림 검증 (시나리오 8.4)

- [ ] **Step 8.1: chart/hello-world-server/values.yaml에 의도적으로 잘못된 image 추가**

`chart/hello-world-server/values.yaml`을 다음과 같이 변경:

```yaml
image:
  name_tag: ""   # 빈 image — helm template 통과하지만 sync 시 validation 실패
replicaCount: 1
```

또는 helm template syntax 자체를 깨뜨리려면 `templates/deployment.yaml`을 수정해도 됨. 가장 확실한 방법은 image 필드를 빈 문자열로.

- [ ] **Step 8.2: commit + push**

```bash
git add chart/hello-world-server/values.yaml
git commit -m "test: invalid empty image to trigger sync-failed"
git push
```

- [ ] **Step 8.3: refresh 후 manual sync 시도**

```bash
argocd app get hello-world-server --refresh
argocd app sync hello-world-server
```

Expected: sync 명령이 실패하거나 sync 작업이 `Failed` 상태로 끝남.

- [ ] **Step 8.4: operationState.phase 확인**

Run:
```bash
kubectl get application hello-world-server -n argocd -o jsonpath='{.status.operationState.phase}'; echo
kubectl get application hello-world-server -n argocd -o jsonpath='{.status.operationState.message}'; echo
```
Expected: `Failed` 또는 `Error`, 그리고 메시지에 실패 원인 (image 관련 에러).

- [ ] **Step 8.5: ~60초 내 webhook-receiver 로그에 `event: argocd.sync-failed` 도착**

Step 6.1 터미널에서 다음 JSON 출력 확인:

```json
{
  ...
  "body": {
    "event": "argocd.sync-failed",
    "severity": "error",
    "application": { "name": "hello-world-server", ... },
    "operation": {
      "phase": "Failed",
      "message": "...image...",
      "startedAt": "...",
      "finishedAt": "..."
    },
    ...
  }
}
```

검증:
- `event: argocd.sync-failed`
- `severity: error`
- `operation.phase: Failed` 또는 `Error`
- `operation.message`에 실패 원인 포함

- [ ] **Step 8.6: 복구 (git revert)**

```bash
git revert HEAD --no-edit
git push
argocd app get hello-world-server --refresh
argocd app sync hello-world-server
```

검증: `kubectl get application hello-world-server -n argocd -o jsonpath='{.status.sync.status}'` → `Synced`.

---

## Task 9: Health Degraded 알림 검증 (시나리오 8.5)

- [ ] **Step 9.1: chart/hello-world-server/values.yaml의 image tag를 존재하지 않는 값으로 변경**

`chart/hello-world-server/values.yaml`:

```yaml
image:
  name_tag: kenshin579/hello-world-server:nonexistent-tag-v999
replicaCount: 1
```

- [ ] **Step 9.2: commit + push + sync**

```bash
git add chart/hello-world-server/values.yaml
git commit -m "test: nonexistent image tag to trigger health-degraded"
git push
argocd app get hello-world-server --refresh
argocd app sync hello-world-server
```

Expected: sync는 성공하지만 (manifest 자체는 valid), 이후 Pod이 ImagePullBackOff로 진입.

- [ ] **Step 9.3: Pod이 ImagePullBackOff 상태로 진입 대기**

```bash
for i in $(seq 1 30); do
  state=$(kubectl get pod -n argocd-noti-test -l app=hello-world-server -o jsonpath='{.items[0].status.containerStatuses[0].state}' 2>/dev/null)
  echo "[$i] $state"
  echo "$state" | grep -q "ImagePullBackOff\|ErrImagePull" && break
  sleep 5
done
```
Expected: `ImagePullBackOff` 또는 `ErrImagePull`.

- [ ] **Step 9.4: Application health 상태 확인**

```bash
sleep 30   # health controller가 health 평가하는 시간 대기
kubectl get application hello-world-server -n argocd -o jsonpath='{.status.health.status}'; echo
```
Expected: `Degraded`.

- [ ] **Step 9.5: ~60초 내 webhook-receiver 로그에 `event: argocd.health-degraded` 도착**

Step 6.1 터미널에서 다음 JSON 출력 확인:

```json
{
  ...
  "body": {
    "event": "argocd.health-degraded",
    "severity": "warning",
    "application": { "name": "hello-world-server", ... },
    "health": { "status": "Degraded" },
    "resources": [
      {
        "kind": "Deployment",
        "name": "hello-world-server",
        "status": "Degraded",
        "message": "..."
      }
    ],
    ...
  }
}
```

검증:
- `event: argocd.health-degraded`
- `health.status: Degraded`
- `resources` 배열에 비정상 리소스 정보 (kind/name/status/message)

- [ ] **Step 9.6: 복구 (git revert)**

```bash
git revert HEAD --no-edit
git push
argocd app get hello-world-server --refresh
argocd app sync hello-world-server
```

검증: `kubectl get application hello-world-server -n argocd -o jsonpath='{.status.health.status}'` → `Healthy`.

---

## Task 10: Negative test — 다른 namespace는 알림 안 옴 (시나리오 8.6)

- [ ] **Step 10.1: 기존 argocd-test의 echo-server에 drift 유발**

이 단계는 기존 ApplicationSet(`appset-list.yaml`)이 이미 배포되어 있어야 한다. 만약 배포되지 않았다면:
```bash
kubectl apply -f bootstrap/application-set/appset-list.yaml
```
배포 후 `kubectl get pod -n argocd-test`로 echo-server가 Running인지 확인.

drift 유발:
```bash
kubectl scale deployment echo-server -n argocd-test --replicas=3
```

- [ ] **Step 10.2: webhook-receiver 로그 모니터링 (90초)**

Step 6.1 터미널 또는 다음 명령으로 90초 동안 새 로그 모니터링:
```bash
kubectl logs --since=90s deployment/webhook-receiver -n argocd-noti-receiver | grep -E "echo-server|event"
```
Expected: 출력 없음 (echo-server는 `argocd-test` ns이므로 trigger 표현식의 namespace 필터에 걸려 webhook 호출 없음). selfHeal로 인해 매우 짧은 OutOfSync 가능하나, 알림은 트리거되지 않아야 함.

- [ ] **Step 10.3: 복구 (필요 시)**

argocd-test의 echo-server는 selfHeal=true라 자동 복구되었을 것:
```bash
kubectl get deployment echo-server -n argocd-test -o jsonpath='{.spec.replicas}'; echo
```
Expected: `1` (selfHeal로 자동 원복).

---

## Task 11: README.md 업데이트

**Files:**
- Modify: `README.md`

- [ ] **Step 11.1: README.md 끝에 알림 시스템 섹션 추가**

`README.md` 마지막에 다음 섹션 추가 (기존 내용 보존):

````markdown

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
````

- [ ] **Step 11.2: 인코딩 확인 (UTF-8)**

Run: `file -I README.md`
Expected: `text/plain; charset=utf-8`.

- [ ] **Step 11.3: targetRevision 임시 변경 원복 + .bak 파일 정리**

Task 5에서 `sed -i.bak`로 만든 `.bak` 파일을 제거하고, targetRevision을 `HEAD`로 되돌린다 (PR merge 후엔 HEAD가 main을 가리키므로).

Run:
```bash
sed -i.tmp 's|targetRevision: feat/argocd-notifications-outofsync|targetRevision: HEAD|g' \
  bootstrap/notifications.yaml \
  bootstrap/application-set/appset-noti-test.yaml
rm -f bootstrap/notifications.yaml.bak bootstrap/application-set/appset-noti-test.yaml.bak
rm -f bootstrap/notifications.yaml.tmp bootstrap/application-set/appset-noti-test.yaml.tmp
```

Run: `grep -n targetRevision bootstrap/notifications.yaml bootstrap/application-set/appset-noti-test.yaml`
Expected: 3개 라인 모두 `targetRevision: HEAD`.

- [ ] **Step 11.4: Commit + push**

```bash
git add README.md bootstrap/notifications.yaml bootstrap/application-set/appset-noti-test.yaml
git commit -m "[feat/argocd-notifications-outofsync] README에 알림 설정 섹션 추가

* 구성/설치/수신 확인/cleanup 가이드
* targetRevision을 HEAD로 원복"
git push
```

---

## Task 12: PR 생성

- [ ] **Step 12.1: PR 생성 (gh CLI + HEREDOC)**

```bash
gh pr create --title "feat: ArgoCD Notifications OutOfSync / Sync Failed / Health Degraded 알림 추가" --body "$(cat <<'EOF'
## Summary
- `argocd-noti-test` namespace에 배포되는 모든 Application의 주요 lifecycle 이벤트를 webhook으로 알림 받는 로컬 테스트 환경 구축
- 3개 trigger: `on-sync-status-out-of-sync` (커스텀), `on-sync-failed`, `on-health-degraded`
- 별도 Helm chart 2개로 알림 설정/수신 서버 분리 — 기존 ArgoCD Helm release 무영향
- bootstrap은 multi-doc YAML 1개 파일로 단순화

## Architecture
- `chart/argocd-notifications-config/` — argocd-notifications-cm/secret을 정의하는 Helm chart
- `chart/webhook-receiver/` — mendhak/http-https-echo 기반 수신 서버 Helm chart
- `bootstrap/notifications.yaml` — 두 chart를 sync하는 Application (multi-doc)
- `bootstrap/application-set/appset-noti-test.yaml` — 알림 테스트 대상 ApplicationSet (`automated` 없음)
- 자세한 설계: `docs/superpowers/specs/2026-05-15-argocd-notifications-outofsync-design.md`

## Test plan
- [x] 시나리오 8.1 정상 설치 검증
- [x] 시나리오 8.2 Cluster Drift → OutOfSync 알림
- [x] 시나리오 8.3 Git Drift → OutOfSync 알림 (새 revision)
- [x] 시나리오 8.4 Sync Failed → sync-failed 알림
- [x] 시나리오 8.5 Health Degraded → health-degraded 알림
- [x] 시나리오 8.6 다른 namespace의 drift는 알림 안 옴 (negative)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

PR URL을 받으면 사용자에게 전달.

---

## Done

모든 task 완료 시:
- argocd-noti-test namespace의 모든 Application에서 OutOfSync / Sync Failed / Health Degraded 이벤트 발생 시 webhook-receiver로 알림 도착
- `kubectl logs -f deployment/webhook-receiver -n argocd-noti-receiver`로 payload JSON 확인 가능
- 6가지 검증 시나리오 통과
- README + spec + plan 문서화 완료
