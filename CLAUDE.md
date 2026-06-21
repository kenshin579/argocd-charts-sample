# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

ArgoCD Charts Sample - ArgoCD와 Helm Chart를 사용한 GitOps 샘플 프로젝트. kind 클러스터(argo-cluster)에서 ArgoCD를 설치하고 ApplicationSet 패턴으로 애플리케이션을 관리한다.

## Commands

### Terraform (ArgoCD 설치)

```bash
cd terraform
make kind-create     # kind 클러스터(argo-cluster) 생성
make tf-infra        # ArgoCD 설치 (init + plan + apply)
make tf-init         # Terraform 초기화
make tf-validate     # 설정 검증
make tf-destroy      # ArgoCD 삭제
make kind-delete     # kind 클러스터 삭제
make tf-clean        # Terraform state 정리
```

### ApplicationSet 배포

```bash
# List Generator (권장)
kubectl apply -f bootstrap/application-set/appset-list.yaml -n argocd

# Git Generator (chart 폴더 자동 탐지)
kubectl apply -f bootstrap/application-set/appset.yaml -n argocd

# Matrix Generator (다중 환경)
kubectl apply -f bootstrap/application-set/appset-matrix.yaml -n argocd
```

### ArgoCD 접속

```bash
# port-forward (별도 터미널에서 실행)
make argocd-port-forward

# UI 접속: https://localhost:8080 (자체 서명 인증서 경고는 무시)

# 저장된 admin 비밀번호(bcrypt 해시) 확인 — 로그인 평문은 argocd_password 변수 값
make view-argocd-password
```

### 리소스 확인

```bash
kubectl get applicationset -n argocd
kubectl get application -n argocd
```

## Architecture

```
├── terraform/           # ArgoCD 설치 (Helm provider)
│   └── modules/infra/   # argocd namespace + helm release
├── bootstrap/           # ArgoCD Application 정의
│   ├── application-set/ # ApplicationSet 패턴 (권장)
│   ├── app-of-apps/     # App of Apps 패턴
│   └── single-app/      # 단일 Application
├── chart/               # Helm Charts
│   ├── echo-server/
│   ├── hello-world-server/
│   ├── hello-world-server-hook/  # ArgoCD Sync Hook 포함
│   └── environments/    # 공유 values 파일
└── app/                 # 애플리케이션 소스 코드
```

## ApplicationSet Generator 선택

- **List Generator** (`appset-list.yaml`): 명시적 목록, 세밀한 제어
- **Git Generator** (`appset.yaml`): chart 폴더 자동 탐지
- **Matrix Generator** (`appset-matrix.yaml`): 다중 환경(dev/staging/prod) 배포

## Git Commit Convention

커밋 메시지는 한국어로 작성하며, 이슈 번호로 시작:

```
[#이슈번호] <설명>

예: [#3000] 주식 가격 조회 API 구현
예: [#3001] fix: 주식 정보 조회 시 발생하는 NullPointerException 수정
```

커밋 타입: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`
