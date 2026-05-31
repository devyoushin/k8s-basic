## 1. 개요 및 비유
**RBAC 심화**는 기본 Role/Binding 패턴을 넘어, 실무 멀티팀 환경에서의 권한 위임, Aggregation, OIDC 연동, Audit 로그 분석까지 다루는 심층 가이드입니다.

💡 **비유하자면 '대기업 보안 출입 시스템'과 같습니다.**
단순히 사원증 발급(Role 생성)에서 끝나지 않고, 부서별 권한 템플릿 조합(Aggregated ClusterRole), 협력사 직원 임시 출입(OIDC/외부 IdP), 누가 언제 어느 문을 열었는지 기록(Audit Log)하는 전체 체계를 관리합니다.

---

## 2. 핵심 설명

### 1) Aggregated ClusterRole — 권한 자동 합산

`aggregationRule`을 사용하면 여러 ClusterRole의 권한을 **자동으로 병합**합니다. 레이블로 선택하므로, 새로운 ClusterRole을 추가하기만 해도 자동으로 합산됩니다.

```
[monitoring-base]  +  [monitoring-alerts]  +  [monitoring-dashboards]
        └────────────────────────────────────────────┘
                         ↓ 자동 병합
                 [monitoring-full] (aggregated)
```

### 2) ClusterRole vs RoleBinding 조합 전략

| 조합 | 효과 | 사용 사례 |
|---|---|---|
| `ClusterRole` + `ClusterRoleBinding` | 클러스터 전체 | 클러스터 관리자, 모니터링 |
| `ClusterRole` + `RoleBinding` | 특정 네임스페이스 | 공통 역할 재사용, 멀티팀 분리 |
| `Role` + `RoleBinding` | 특정 네임스페이스 | 완전한 네임스페이스 격리 |

> **핵심 패턴:** 공통 권한은 ClusterRole로 정의하고, RoleBinding으로 네임스페이스 범위로 제한합니다. 팀별 네임스페이스 분리와 공통 역할 재사용을 동시에 달성합니다.

### 3) OIDC 연동 — 외부 IdP로 사용자 인증

쿠버네티스는 자체 유저 DB가 없습니다. `--oidc-issuer-url`, `--oidc-client-id`를 API Server에 설정하면 Google, Okta, Keycloak 등 외부 IdP 토큰으로 인증할 수 있습니다.

```
[사용자] --로그인→ [Keycloak/Okta] --JWT 발급→ [kubectl --token=JWT] --검증→ [API Server]
                                                                              ↓
                                                                    JWT의 groups claim으로
                                                                    RoleBinding 매핑
```

### 4) Impersonation — 권한 위임 테스트

관리자가 다른 유저/그룹 권한으로 실행해볼 수 있는 기능입니다. CI/CD 파이프라인이나 권한 검증 자동화에 활용합니다.

```bash
kubectl get pods --as=developer@example.com
kubectl get pods --as-group=team-backend --as=any-user
```

### 5) Audit Log — 권한 사용 추적

API Server의 Audit 정책으로 "누가 언제 무슨 API를 호출했는지" 기록합니다. 보안 감사 및 침해 사고 분석의 핵심입니다.

**Audit Level:**

| Level | 기록 내용 |
|---|---|
| `None` | 기록 안 함 |
| `Metadata` | 요청 메타데이터만 (user, resource, verb) |
| `Request` | 메타데이터 + 요청 바디 |
| `RequestResponse` | 메타데이터 + 요청/응답 바디 전체 |

---

## 3. YAML 적용 예시

### Aggregated ClusterRole
```yaml
# 베이스 역할들 (레이블로 집합 표시)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: monitoring-pods
  labels:
    rbac.example.com/aggregate-to-monitoring: "true"  # 집합 레이블
rules:
- apiGroups: [""]
  resources: ["pods", "pods/log", "pods/exec"]
  verbs: ["get", "list", "watch"]

---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: monitoring-metrics
  labels:
    rbac.example.com/aggregate-to-monitoring: "true"
rules:
- apiGroups: ["metrics.k8s.io"]
  resources: ["pods", "nodes"]
  verbs: ["get", "list"]

---
# 위 두 ClusterRole을 자동으로 합산하는 Aggregated ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: monitoring-full
aggregationRule:
  clusterRoleSelectors:
  - matchLabels:
      rbac.example.com/aggregate-to-monitoring: "true"  # 레이블 셀렉터로 자동 병합
rules: []  # aggregationRule 사용 시 rules는 비워둠 (자동으로 채워짐)
```

### 멀티팀 네임스페이스 격리 패턴
```yaml
# 공통 개발자 역할 (ClusterRole로 한 번만 정의)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: developer-base
rules:
- apiGroups: ["", "apps", "batch"]
  resources: ["pods", "deployments", "services", "jobs", "cronjobs", "configmaps"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
- apiGroups: [""]
  resources: ["pods/log", "pods/exec", "pods/portforward"]
  verbs: ["get", "create"]
# 삭제 권한은 의도적으로 제외 (실수 방지)

---
# 팀A 네임스페이스에만 적용
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: team-a-developers
  namespace: team-a          # 이 네임스페이스에만 권한 적용
subjects:
- kind: Group
  name: team-a-group         # OIDC groups claim과 매핑
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole          # ClusterRole을 네임스페이스 범위로 제한
  name: developer-base
  apiGroup: rbac.authorization.k8s.io

---
# 팀B 네임스페이스에도 동일 ClusterRole 재사용
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: team-b-developers
  namespace: team-b
subjects:
- kind: Group
  name: team-b-group
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: developer-base
  apiGroup: rbac.authorization.k8s.io
```

### Audit Policy 설정 (API Server 인수)
```yaml
# /etc/kubernetes/audit-policy.yaml
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
# Secret 접근은 RequestResponse로 전체 기록 (보안 감사 필수)
- level: RequestResponse
  resources:
  - group: ""
    resources: ["secrets"]

# ServiceAccount 토큰 생성도 RequestResponse 기록
- level: RequestResponse
  verbs: ["create"]
  resources:
  - group: ""
    resources: ["serviceaccounts/token"]

# 일반 읽기 작업은 Metadata만 기록 (로그 용량 절약)
- level: Metadata
  verbs: ["get", "list", "watch"]
  resources:
  - group: ""
    resources: ["pods", "deployments", "services"]

# 시스템 SA의 반복 요청은 로깅 제외 (노이즈 제거)
- level: None
  users:
  - system:kube-controller-manager
  - system:kube-scheduler
  verbs: ["get", "list", "watch"]

# 나머지 모든 요청은 Metadata 기록
- level: Metadata
```

```bash
# kube-apiserver에 Audit 활성화 (kubeadm 환경)
# /etc/kubernetes/manifests/kube-apiserver.yaml 에 추가:
# --audit-policy-file=/etc/kubernetes/audit-policy.yaml
# --audit-log-path=/var/log/kubernetes/audit/audit.log
# --audit-log-maxage=30        # 30일 보관
# --audit-log-maxbackup=10     # 최대 10개 파일 순환
# --audit-log-maxsize=100      # 100MB마다 파일 분할
```

### OIDC 연동 (Keycloak 예시)
```bash
# API Server 설정 인수
--oidc-issuer-url=https://keycloak.example.com/realms/k8s
--oidc-client-id=kubernetes
--oidc-username-claim=email        # JWT의 어느 claim을 username으로 쓸지
--oidc-groups-claim=groups         # JWT의 어느 claim을 group으로 쓸지
--oidc-username-prefix=oidc:       # username 충돌 방지 접두사
--oidc-groups-prefix=oidc:
```

```yaml
# OIDC groups claim 기반 RoleBinding
# JWT payload: {"email": "dev@company.com", "groups": ["k8s-developers", "team-a"]}
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: oidc-team-a
  namespace: team-a
subjects:
- kind: Group
  name: "oidc:team-a"             # --oidc-groups-prefix 반영
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: developer-base
  apiGroup: rbac.authorization.k8s.io
```

---

## 4. 트러블 슈팅

* **Aggregated ClusterRole 권한이 반영 안 됨:**
  * `kubectl get clusterrole <name> -o yaml`로 `rules` 필드가 자동으로 채워졌는지 확인하세요.
  * 레이블이 정확히 일치하는지, `matchLabels` 값이 "true" 문자열인지 확인합니다.

* **OIDC 인증 후 `Unauthorized` 에러:**
  * JWT 만료 여부 확인: `kubectl get pods --token=$(cat token.jwt)`으로 테스트합니다.
  * `--oidc-username-claim`, `--oidc-groups-claim` 설정과 JWT payload의 claim 이름이 일치하는지 확인합니다.
  * API Server 로그: `journalctl -u kube-apiserver | grep "OIDC"`

* **Audit Log에서 의심스러운 접근 탐지:**
  ```bash
  # Secret 조회 이력 확인
  cat /var/log/kubernetes/audit/audit.log | jq '. | select(.objectRef.resource == "secrets" and .verb == "get")'

  # 특정 유저의 전체 API 호출 이력
  cat audit.log | jq '. | select(.user.username == "suspicious@example.com")'

  # 실패한 요청만 필터링
  cat audit.log | jq '. | select(.responseStatus.code >= 400)'
  ```

* **최소 권한 원칙 점검 자동화:**
  ```bash
  # 특정 SA가 실제로 사용하는 권한 확인 (rbac-tool 활용)
  kubectl auth can-i --list --as=system:serviceaccount:default:my-app-sa

  # 클러스터 전체 ClusterRoleBinding 목록 — cluster-admin 오남용 확인
  kubectl get clusterrolebindings -o json | jq '.items[] | select(.roleRef.name == "cluster-admin") | .subjects'
  ```

* **ServiceAccount 토큰 자동 마운트 비활성화 (보안 강화):**
  ```yaml
  # 토큰이 필요 없는 파드는 마운트 차단
  spec:
    automountServiceAccountToken: false
    containers:
    - name: app
      image: my-app:1.0
  ```
