## 1. 개요 및 비유

GitOps는 Git 저장소를 클러스터의 **단일 진실 공급원(Single Source of Truth)**으로 사용하는 운영 패턴입니다. ArgoCD는 Git 상태와 클러스터 상태를 지속적으로 동기화합니다.

💡 **비유하자면 건물 설계도(Git)와 실제 건물(클러스터)을 항상 일치시키는 자동화 시스템과 같습니다.**
누군가 설계도 없이 건물을 수정하면(kubectl 직접 변경) 시스템이 감지하고 설계도대로 되돌립니다.

---

## 2. ArgoCD 아키텍처

### 2.1 핵심 컴포넌트

```
┌────────────────────────────────────────────────────────────┐
│  ArgoCD (클러스터 내 설치)                                  │
│                                                            │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────────┐   │
│  │  API Server  │  │  Repo Server │  │  Application   │   │
│  │  (argocd-    │  │  (Git 저장소 │  │  Controller    │   │
│  │   server)    │  │   캐시/렌더링)│  │  (Reconcile)   │   │
│  └──────┬───────┘  └──────┬───────┘  └───────┬────────┘   │
│         │                 │                   │            │
│  ┌──────▼───────────────────────────────────▼───────────┐  │
│  │  Redis (상태 캐시)                                    │  │
│  └──────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────┘
         │                  │                   │
         ▼                  ▼                   ▼
    ArgoCD UI/CLI       Git 저장소           Kubernetes API
    (사용자 접근)     (Helm/Kustomize/YAML)   (클러스터 적용)
```

### 2.2 Sync 동작 원리

```
ArgoCD Application Controller 루프 (기본 3분마다):
        │
        ▼
1. Repo Server: Git에서 원하는 상태 계산
   - Helm template 렌더링
   - Kustomize 빌드
   - 순수 YAML 로드
        │
        ▼
2. Live State 조회 (클러스터 현재 상태)
   - API Server에서 리소스 조회
   - Informer 캐시 활용
        │
        ▼
3. 두 상태 Diff 계산
   - Synced:       Git == Cluster
   - OutOfSync:    Git != Cluster
        │
        ▼
4. Auto Sync 설정이면:
   - OutOfSync 감지 즉시 Apply 수행
   - kubectl apply와 동일한 동작
   - 서버사이드 어플라이(SSA) 사용
```

---

## 3. Application 설정 패턴

### 3.1 기본 Application

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: my-app
  namespace: argocd
  finalizers:
  - resources-finalizer.argocd.argoproj.io  # App 삭제 시 리소스도 삭제
spec:
  project: default

  # Git 소스
  source:
    repoURL: https://github.com/myorg/k8s-manifests.git
    targetRevision: HEAD    # 브랜치, 태그, 커밋 SHA 모두 가능
    path: apps/my-app       # 저장소 내 경로

  # 배포 대상 클러스터 & 네임스페이스
  destination:
    server: https://kubernetes.default.svc   # 현재 클러스터
    namespace: production

  # 동기화 정책
  syncPolicy:
    automated:
      prune: true      # Git에서 삭제된 리소스 클러스터에서도 삭제
      selfHeal: true   # 클러스터 직접 변경 시 Git으로 자동 복구
    syncOptions:
    - CreateNamespace=true      # 네임스페이스 자동 생성
    - ServerSideApply=true      # SSA 사용 (충돌 감소)
    - PruneLast=true            # 리소스 생성/업데이트 후 삭제 수행
    retry:
      limit: 5
      backoff:
        duration: 5s
        maxDuration: 3m
        factor: 2
```

### 3.2 Helm Application

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: prometheus-stack
  namespace: argocd
spec:
  source:
    repoURL: https://prometheus-community.github.io/helm-charts
    chart: kube-prometheus-stack
    targetRevision: 55.5.0      # Helm 차트 버전 고정
    helm:
      releaseName: prometheus
      values: |
        grafana:
          enabled: true
          adminPassword: my-secret-password
        prometheus:
          prometheusSpec:
            retention: 30d
            storageSpec:
              volumeClaimTemplate:
                spec:
                  storageClassName: gp3
                  resources:
                    requests:
                      storage: 50Gi
      # 또는 외부 values 파일 참조
      valueFiles:
      - values-production.yaml
  destination:
    server: https://kubernetes.default.svc
    namespace: monitoring
```

### 3.3 App of Apps 패턴 — 대규모 관리

```
Git 저장소 구조:
apps/
├── root-app.yaml          ← ArgoCD에 이 하나만 등록
├── team-alpha/
│   ├── application.yaml   ← ArgoCD Application 정의
│   └── values.yaml
└── team-beta/
    └── application.yaml

root-app.yaml이 apps/ 디렉토리를 바라보며
  → team-alpha Application 생성
  → team-beta Application 생성
  → 각 Application이 실제 서비스 배포
```

```yaml
# root-app.yaml (부트스트랩 앱)
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: root-app
  namespace: argocd
spec:
  source:
    repoURL: https://github.com/myorg/k8s-manifests.git
    targetRevision: HEAD
    path: apps                  # 모든 Application 정의 위치
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd           # Application 리소스는 argocd 네임스페이스에
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```

---

## 4. ApplicationSet — 동적 Application 생성

하나의 템플릿으로 여러 클러스터/환경에 동일한 앱을 배포합니다.

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: my-app-all-envs
  namespace: argocd
spec:
  generators:
  # 방법 1: List — 환경별 명시
  - list:
      elements:
      - cluster: dev
        url: https://dev-cluster.example.com
        namespace: my-app
        valuesFile: values-dev.yaml
      - cluster: staging
        url: https://staging-cluster.example.com
        namespace: my-app
        valuesFile: values-staging.yaml
      - cluster: production
        url: https://prod-cluster.example.com
        namespace: my-app
        valuesFile: values-prod.yaml

  # 방법 2: Cluster — 등록된 모든 클러스터에 자동 배포
  # - clusters: {}

  # 방법 3: Git Directory — Git 폴더 구조로 자동 생성
  # - git:
  #     repoURL: https://github.com/myorg/manifests.git
  #     directories:
  #     - path: clusters/*

  template:
    metadata:
      name: "my-app-{{cluster}}"
    spec:
      project: default
      source:
        repoURL: https://github.com/myorg/k8s-manifests.git
        targetRevision: HEAD
        path: apps/my-app
        helm:
          valueFiles:
          - "{{valuesFile}}"
      destination:
        server: "{{url}}"
        namespace: "{{namespace}}"
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
```

---

## 5. 롤백과 배포 전략

```bash
# 현재 앱 상태 확인
argocd app get my-app
argocd app history my-app
# 출력:
# ID   DATE                REVISION
# 0    2024-01-01 10:00   HEAD (abc1234)
# 1    2024-01-02 09:00   HEAD (def5678)   ← 현재

# 이전 버전으로 롤백
argocd app rollback my-app 0   # ID 0번으로

# 특정 Git 커밋으로 배포
argocd app set my-app --revision abc1234
argocd app sync my-app

# Sync 강제 실행 (캐시 무시)
argocd app sync my-app --force

# Sync 일시 중지 (자동 Sync 비활성화)
argocd app set my-app --sync-policy none
```

---

## 6. ArgoCD RBAC & 프로젝트

```yaml
# AppProject — 팀별 배포 권한과 범위 제한
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: team-alpha
  namespace: argocd
spec:
  description: Team Alpha 전용 프로젝트

  # 허용된 Git 소스
  sourceRepos:
  - https://github.com/myorg/team-alpha-manifests.git

  # 허용된 배포 대상
  destinations:
  - namespace: team-alpha-*    # 와일드카드 가능
    server: https://kubernetes.default.svc

  # 허용된 리소스 종류
  namespaceResourceWhitelist:
  - group: apps
    kind: Deployment
  - group: ""
    kind: Service
  # ClusterRole, ClusterRoleBinding 등 클러스터 레벨 리소스는 금지

  # RBAC 역할 정의
  roles:
  - name: developer
    description: 팀 개발자 (배포 가능, 삭제 불가)
    policies:
    - p, proj:team-alpha:developer, applications, get, team-alpha/*, allow
    - p, proj:team-alpha:developer, applications, sync, team-alpha/*, allow
    groups:
    - team-alpha-developers   # SSO 그룹과 연동
```

---

## 7. 트러블슈팅

* **OutOfSync 상태에서 Sync 실패:**
  ```bash
  argocd app sync my-app --debug

  # 흔한 원인: 리소스 충돌 (다른 곳에서 이미 관리 중)
  # → 해당 리소스에 어노테이션 추가
  kubectl annotate <resource> <name> \
    argocd.argoproj.io/managed-by=my-app

  # ServerSideApply 충돌인 경우
  argocd app sync my-app --server-side --force-conflicts
  ```

* **Git 저장소 연결 실패:**
  ```bash
  # 저장소 연결 상태 확인
  argocd repo list
  argocd repo get https://github.com/myorg/manifests.git

  # SSH 키 재등록
  argocd repo add https://github.com/myorg/manifests.git \
    --username myuser \
    --password mytoken  # GitHub Personal Access Token
  ```

* **자동 Sync가 무한 루프:**
  ```bash
  # 리소스가 계속 변경되는 경우 (예: 컨트롤러가 필드 자동 추가)
  # → 해당 필드를 Ignore 설정
  argocd app set my-app \
    --resource-ignore-differences '{"group":"apps","kind":"Deployment","jsonPointers":["/spec/replicas"]}'
  ```
