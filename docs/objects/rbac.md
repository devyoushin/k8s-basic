## 1. 개요 및 비유
**RBAC(Role-Based Access Control, 역할 기반 접근 제어)**는 "누가(Subject) 어떤 리소스(Resource)에 어떤 작업(Verb)을 할 수 있는지"를 정의하는 쿠버네티스의 권한 관리 시스템입니다. 4가지 오브젝트(`Role`, `ClusterRole`, `RoleBinding`, `ClusterRoleBinding`)로 구성됩니다.

💡 **비유하자면 '사원증 등급 시스템'과 같습니다.**
일반 직원(ServiceAccount/User)은 자기 팀 공간(Namespace)에만 출입 가능하고, 팀장(ClusterRole)은 모든 층(Cl러스터 전체)을 돌아다닐 수 있습니다. 사원증(RoleBinding)에 등급을 부여하는 것이 바인딩 작업입니다.

## 2. 핵심 설명

### 4가지 RBAC 오브젝트

| 오브젝트 | 범위 | 설명 |
|---|---|---|
| `Role` | 특정 Namespace | 해당 네임스페이스 내 권한 정의 |
| `ClusterRole` | 클러스터 전체 | 모든 네임스페이스 또는 클러스터 수준 리소스 권한 정의 |
| `RoleBinding` | 특정 Namespace | Subject에게 Role 또는 ClusterRole을 네임스페이스 범위로 부여 |
| `ClusterRoleBinding` | 클러스터 전체 | Subject에게 ClusterRole을 클러스터 범위로 부여 |

### Subject(권한을 받는 대상) 3가지
* **User:** 사람이 사용하는 계정 (kubectl 사용자)
* **Group:** 사용자 그룹
* **ServiceAccount:** 파드 내 애플리케이션이 API Server에 접근할 때 사용하는 계정

### 자주 쓰는 Verb(동사)
`get`, `list`, `watch`, `create`, `update`, `patch`, `delete`

## 3. YAML 적용 예시

### Role + RoleBinding (네임스페이스 범위)
```yaml
# 특정 네임스페이스에서 파드 조회만 허용하는 Role
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: pod-reader
  namespace: production
rules:
- apiGroups: [""]          # ""는 core API group (v1)
  resources: ["pods", "pods/log"]
  verbs: ["get", "list", "watch"]

---
# 위 Role을 developer 유저에게 부여
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: developer-pod-reader
  namespace: production
subjects:
- kind: User
  name: developer@example.com
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: Role
  name: pod-reader
  apiGroup: rbac.authorization.k8s.io
```

### ServiceAccount + ClusterRole (파드에서 API Server 접근)
```yaml
# 애플리케이션용 ServiceAccount
apiVersion: v1
kind: ServiceAccount
metadata:
  name: my-app-sa
  namespace: default

---
# 파드가 ConfigMap을 읽을 수 있는 ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: configmap-reader
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]

---
# ServiceAccount에 ClusterRole 부여 (네임스페이스 범위로)
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-app-configmap-reader
  namespace: default
subjects:
- kind: ServiceAccount
  name: my-app-sa
  namespace: default
roleRef:
  kind: ClusterRole
  name: configmap-reader
  apiGroup: rbac.authorization.k8s.io

---
# 파드에서 ServiceAccount 사용
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  template:
    spec:
      serviceAccountName: my-app-sa  # 이 SA의 권한으로 API Server 호출
      containers:
      - name: app
        image: my-app:1.0
```

**권한 확인 명령어:**
```bash
# 현재 사용자가 특정 작업을 할 수 있는지 확인
kubectl auth can-i create pods
kubectl auth can-i delete pods --namespace production

# 특정 유저가 할 수 있는지 확인 (관리자용)
kubectl auth can-i list secrets --namespace kube-system --as developer@example.com
```

## 4. 트러블 슈팅
* **`Error from server (Forbidden)` 에러:**
  * RBAC 권한이 부족한 것입니다. `kubectl auth can-i <verb> <resource>` 로 현재 권한을 확인하고, 필요한 권한을 Role/RoleBinding으로 추가하세요.
* **파드에서 API Server 호출 시 401/403 에러:**
  * 파드 스펙에 `serviceAccountName`이 지정되어 있는지, 해당 SA에 올바른 RoleBinding이 있는지 확인하세요.
  * `automountServiceAccountToken: false` 가 설정된 경우 토큰이 마운트되지 않습니다.
* **최소 권한 원칙 적용 방법:**
  * `ClusterAdmin`이나 광범위한 권한 부여를 피하고, 실제로 필요한 리소스와 동사만 명시하세요.
  * `kubectl auth reconcile -f rbac.yaml` 명령어로 RBAC 설정을 안전하게 적용할 수 있습니다.
