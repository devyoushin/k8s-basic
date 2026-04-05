## 1. 개요 및 비유

멀티테넌시(Multi-tenancy)는 하나의 클러스터를 여러 팀/고객이 안전하게 공유하는 패턴입니다. 완벽한 격리와 효율적인 자원 공유 사이의 균형이 핵심입니다.

💡 **비유하자면 오피스 빌딩 임대 방식의 차이와 같습니다.**
소프트 멀티테넌시는 같은 층을 파티션으로 나눠 쓰는 것(빠르지만 소음 공유). 하드 멀티테넌시는 층 전체를 별도 임차인에게 주는 것(강한 격리). 별도 클러스터는 건물을 따로 짓는 것(완전 격리, 비용 최고).

---

## 2. 격리 수준별 비교

```
격리 강도  낮음 ────────────────────────────────── 높음
          │                                          │
    Namespace 격리    노드 격리      별도 클러스터
          │           (전용 노드)
    - 빠름            - 보통         - 느림
    - 저비용          - 중간 비용    - 고비용
    - 소프트 격리     - 커널 공유    - 완전 격리
    - 실수로 탈출 가능 - 물리적 분리  - 네트워크도 분리
```

---

## 3. Namespace 기반 멀티테넌시

### 3.1 테넌트 격리 체크리스트

```
각 테넌트(팀/프로젝트)에 필요한 격리 요소:
┌────────────────────────────────────────────┐
│ ① 리소스 쿼터 (ResourceQuota)              │
│    → CPU/Memory/PVC/파드 수 제한           │
│                                            │
│ ② 네트워크 정책 (NetworkPolicy)            │
│    → 네임스페이스 간 트래픽 차단            │
│                                            │
│ ③ RBAC                                    │
│    → 다른 네임스페이스 리소스 접근 금지     │
│                                            │
│ ④ Pod Security Standards (PSS)            │
│    → 권한 있는 컨테이너 실행 금지           │
│                                            │
│ ⑤ LimitRange                              │
│    → requests/limits 없는 파드 기본값 설정 │
└────────────────────────────────────────────┘
```

### 3.2 테넌트 셋업 완전 예시

```yaml
# 1. 네임스페이스 생성
apiVersion: v1
kind: Namespace
metadata:
  name: team-alpha
  labels:
    team: alpha
    pod-security.kubernetes.io/enforce: restricted   # PSS 강제 적용

---
# 2. ResourceQuota — 자원 한도
apiVersion: v1
kind: ResourceQuota
metadata:
  name: team-alpha-quota
  namespace: team-alpha
spec:
  hard:
    requests.cpu: "10"
    requests.memory: 20Gi
    limits.cpu: "20"
    limits.memory: 40Gi
    pods: "50"
    persistentvolumeclaims: "20"
    requests.storage: 500Gi
    count/deployments.apps: "20"
    count/services: "30"
    count/secrets: "50"
    count/configmaps: "50"

---
# 3. LimitRange — 기본값 및 최대/최소 강제
apiVersion: v1
kind: LimitRange
metadata:
  name: team-alpha-limits
  namespace: team-alpha
spec:
  limits:
  - type: Container
    default:           # requests/limits 없으면 이 값 자동 적용
      cpu: 200m
      memory: 256Mi
    defaultRequest:
      cpu: 100m
      memory: 128Mi
    max:               # 최대 허용값
      cpu: "4"
      memory: 4Gi
    min:               # 최소 설정값
      cpu: 50m
      memory: 64Mi
  - type: PersistentVolumeClaim
    max:
      storage: 50Gi

---
# 4. NetworkPolicy — 기본 차단 (화이트리스트 방식)
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-all
  namespace: team-alpha
spec:
  podSelector: {}      # 모든 파드에 적용
  policyTypes:
  - Ingress
  - Egress

---
# 네임스페이스 내부 통신 허용
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-same-namespace
  namespace: team-alpha
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          team: alpha   # 같은 팀 네임스페이스만 허용
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          team: alpha
  - to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: kube-system  # DNS 조회 허용
    ports:
    - port: 53
      protocol: UDP

---
# 5. RBAC — 팀 권한 부여
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: team-alpha-admin
  namespace: team-alpha
subjects:
- kind: Group
  name: team-alpha-developers   # SSO/OIDC 그룹
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: admin                  # 해당 네임스페이스의 admin 권한
  apiGroup: rbac.authorization.k8s.io
```

---

## 4. 노드 격리 — 전용 노드 할당

민감한 워크로드나 특정 팀에 물리적 노드를 전용으로 할당합니다.

```bash
# 팀 전용 노드 표시
kubectl label node worker-3 team=alpha
kubectl label node worker-4 team=alpha

# 다른 팀 파드가 못 쓰도록 Taint
kubectl taint node worker-3 team=alpha:NoSchedule
kubectl taint node worker-4 team=alpha:NoSchedule
```

```yaml
# team-alpha 파드만 이 노드에 배치
spec:
  tolerations:
  - key: team
    value: alpha
    effect: NoSchedule
  nodeSelector:
    team: alpha

# 또는 NodeAffinity로 더 세밀하게
spec:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: team
            operator: In
            values: [alpha]
```

---

## 5. HierarchicalNamespace Controller (HNC) — 계층적 네임스페이스

대규모 조직에서 네임스페이스를 트리 구조로 관리합니다.

```
org/                    ← 루트 네임스페이스
├── team-platform/      ← 플랫폼 팀
│   ├── monitoring/     ← 서브 네임스페이스
│   └── logging/
└── team-product/       ← 프로덕트 팀
    ├── frontend/
    └── backend/

HNC 기능:
- 부모 네임스페이스의 Policy, RBAC, NetworkPolicy를 자식에 자동 전파
- 팀 단위 쿼터 + 하위 프로젝트 세부 쿼터 계층 관리
```

```bash
# HNC 설치 후 서브 네임스페이스 생성
kubectl hns create monitoring -n team-platform

# 부모 네임스페이스의 정책 자식에 전파 확인
kubectl hns describe team-platform

# 전파된 리소스 확인
kubectl get rolebindings -n monitoring | grep propagated
```

---

## 6. 가상 클러스터 (vCluster) — 강한 격리

물리 클러스터 위에 완전한 쿠버네티스 API를 갖춘 가상 클러스터를 생성합니다.

```
물리 클러스터 (호스트)
└── vCluster A (team-alpha 전용)
    ├── 전용 API Server
    ├── 전용 etcd (또는 SQLite)
    ├── 전용 Controller Manager
    └── 가상 노드 (실제 파드는 호스트 노드에서 실행)

vCluster B (team-beta 전용)
    └── ...

장점:
- 팀이 cluster-admin 권한 가짐 (CRD 설치 등)
- 완전한 쿠버네티스 API 격리
- 호스트 클러스터의 물리 노드는 공유 (비용 효율)
```

```bash
# vcluster CLI로 가상 클러스터 생성
vcluster create team-alpha -n vcluster-team-alpha

# 가상 클러스터에 접속
vcluster connect team-alpha -n vcluster-team-alpha

# 가상 클러스터 내에서 모든 작업 가능
kubectl get nodes  # 가상 노드 표시
kubectl create namespace my-ns
kubectl apply -f my-crd.yaml  # CRD 설치 가능
```

---

## 7. 트러블슈팅

* **ResourceQuota 초과로 파드 생성 실패:**
  ```bash
  # 쿼터 현재 사용량 확인
  kubectl describe quota -n team-alpha
  # Used와 Hard 비교

  # 어떤 파드가 가장 많이 쓰는지
  kubectl top pods -n team-alpha --sort-by=cpu

  # LimitRange 기본값이 의도치 않게 큰 경우
  kubectl describe limitrange -n team-alpha
  ```

* **NetworkPolicy 설정 후 DNS 안 됨:**
  ```bash
  # kube-system으로의 UDP 53 egress 허용 확인
  kubectl get networkpolicy -n team-alpha -o yaml | grep -A10 egress

  # CoreDNS 파드의 레이블 확인 (namespaceSelector에 맞는 레이블인지)
  kubectl get ns kube-system --show-labels

  # 직접 DNS 테스트
  kubectl exec -n team-alpha my-pod -- nslookup kubernetes.default
  ```

* **RBAC으로 격리했는데 타 네임스페이스 리소스 조회됨:**
  ```bash
  # ClusterRole 바인딩 여부 확인 (ClusterRole은 전체 클러스터에 적용)
  kubectl get clusterrolebinding | grep team-alpha

  # 사용자 권한 시뮬레이션
  kubectl auth can-i get pods --namespace=team-beta \
    --as=user@example.com
  ```
