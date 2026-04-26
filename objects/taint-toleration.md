# Taint & Toleration (테인트 & 톨러레이션)

## 1. 개요 및 비유

노드에 특정 파드만 스케줄링되도록 제한하거나, 파드가 특정 노드의 제약을 허용하도록 설정하는 메커니즘.

💡 **비유**: 노드는 "출입 제한 구역", 테인트(Taint)는 "출입 제한 표지판", 톨러레이션(Toleration)은 "출입 허가증"임. 허가증 없는 파드는 그 구역에 들어갈 수 없음.

---

## 2. 핵심 설명

### 2.1 동작 원리

- **Taint (테인트)**: 노드에 설정하여 특정 파드가 스케줄링되지 않도록 배제 (repel)
- **Toleration (톨러레이션)**: 파드에 설정하여 해당 테인트를 "허용"한다고 선언
- Taint가 있는 노드에는 **일치하는 Toleration을 가진 파드만** 스케줄링 가능

**Taint 효과(Effect) 3가지**:

| Effect | 동작 |
|--------|------|
| `NoSchedule` | Toleration 없는 파드를 스케줄링하지 않음 (기존 파드 유지) |
| `PreferNoSchedule` | 가능하면 스케줄링하지 않음 (소프트 규칙) |
| `NoExecute` | 스케줄링 금지 + 기존 파드도 퇴출(Evict) |

**Taint 키-값-효과 형식**:
```
key=value:effect
```

### 2.2 YAML 적용 예시

**노드에 Taint 추가 (kubectl)**:
```bash
# GPU 전용 노드로 지정
kubectl taint nodes <NODE_NAME> dedicated=gpu:NoSchedule

# 유지보수 모드
kubectl taint nodes <NODE_NAME> maintenance=true:NoExecute

# Taint 제거 (끝에 - 붙임)
kubectl taint nodes <NODE_NAME> dedicated=gpu:NoSchedule-
```

**GPU 전용 파드 (Toleration 설정)**:
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: gpu-workload
  namespace: ml-team
  labels:
    app: gpu-workload
    version: v1
    managed-by: kubectl
spec:
  tolerations:
    # NoSchedule Taint 허용
    - key: "dedicated"
      operator: "Equal"
      value: "gpu"
      effect: "NoSchedule"
  containers:
    - name: gpu-container
      image: nvidia/cuda:<TAG>
      resources:
        requests:
          memory: "2Gi"
          cpu: "500m"
        limits:
          memory: "4Gi"
          cpu: "2000m"
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        readOnlyRootFilesystem: true
        capabilities:
          drop: ["ALL"]
```

**NoExecute + tolerationSeconds (일시 허용)**:
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: tolerant-pod
  namespace: production
  labels:
    app: tolerant-pod
    version: v1
    managed-by: kubectl
spec:
  tolerations:
    # NoExecute 테인트를 최대 300초 동안 허용 후 퇴출
    - key: "maintenance"
      operator: "Equal"
      value: "true"
      effect: "NoExecute"
      tolerationSeconds: 300
  containers:
    - name: app
      image: <IMAGE>:<TAG>
      resources:
        requests:
          memory: "128Mi"
          cpu: "100m"
        limits:
          memory: "256Mi"
          cpu: "500m"
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        readOnlyRootFilesystem: true
        capabilities:
          drop: ["ALL"]
```

**모든 Taint 허용 (Operator: Exists)**:
```yaml
spec:
  tolerations:
    # key 지정 없이 모든 테인트 허용 (DaemonSet에서 주로 사용)
    - operator: "Exists"
```

> ⚠️ `operator: Exists` + key 미지정은 노드의 모든 Taint를 무시함. DaemonSet처럼 모든 노드에 반드시 배포해야 할 경우에만 사용.

**DaemonSet 예시 (시스템 파드가 모든 노드에 배포되어야 할 때)**:
```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: log-collector
  namespace: monitoring
  labels:
    app: log-collector
    version: v1
    managed-by: kubectl
spec:
  selector:
    matchLabels:
      app: log-collector
  template:
    metadata:
      labels:
        app: log-collector
        version: v1
        managed-by: kubectl
    spec:
      tolerations:
        # 마스터/컨트롤플레인 노드 Taint 허용
        - key: "node-role.kubernetes.io/control-plane"
          operator: "Exists"
          effect: "NoSchedule"
        # 노드 상태 이상 시에도 유지 (node.kubernetes.io/not-ready)
        - key: "node.kubernetes.io/not-ready"
          operator: "Exists"
          effect: "NoExecute"
          tolerationSeconds: 300
        - key: "node.kubernetes.io/unreachable"
          operator: "Exists"
          effect: "NoExecute"
          tolerationSeconds: 300
      containers:
        - name: log-collector
          image: fluentd:<TAG>
          resources:
            requests:
              memory: "128Mi"
              cpu: "100m"
            limits:
              memory: "256Mi"
              cpu: "500m"
          securityContext:
            runAsNonRoot: true
            runAsUser: 1000
            readOnlyRootFilesystem: true
            capabilities:
              drop: ["ALL"]
```

### 2.3 Best Practice

- **목적별 노드 분리**: GPU 노드, 고메모리 노드, 스팟 인스턴스 등 용도에 맞게 Taint 설정
- **Node Affinity와 함께 사용**: Taint/Toleration은 "배제"만 담당. 특정 노드에 "유인"하려면 `nodeAffinity`도 같이 설정
- **NoExecute + tolerationSeconds**: 유지보수 시 점진적 파드 퇴출에 활용 (graceful drain)
- **DaemonSet Toleration**: 시스템 데몬 파드는 컨트롤플레인 노드 Taint도 허용해야 모든 노드에 배포됨
- **Taint 남용 금지**: 너무 많은 Taint는 스케줄링 복잡도 증가 → 파드가 Pending 상태에 빠지는 원인

---

## 3. 트러블슈팅

### 3.1 주요 이슈

#### 파드가 Pending 상태 — Taint 때문에 스케줄링 불가

**증상**: `kubectl get pods` 에서 파드가 계속 `Pending` 상태

**원인**: 파드에 Toleration이 없어 모든 노드의 Taint를 통과하지 못함

**해결 방법**:
```bash
# 파드 이벤트에서 Taint 관련 메시지 확인
kubectl describe pod <POD_NAME> -n <NAMESPACE>
# Events 섹션: "0/3 nodes are available: 3 node(s) had untolerated taint"

# 현재 노드에 설정된 Taint 목록 확인
kubectl get nodes -o custom-columns=NAME:.metadata.name,TAINTS:.spec.taints

# 특정 노드의 Taint 상세 확인
kubectl describe node <NODE_NAME> | grep -A5 Taints
```

파드 스펙에 누락된 Toleration 추가:
```yaml
spec:
  tolerations:
    - key: "dedicated"
      operator: "Equal"
      value: "gpu"
      effect: "NoSchedule"
```

#### NoExecute로 파드가 갑자기 퇴출됨

**증상**: 실행 중이던 파드가 갑자기 `Terminating` → 삭제됨

**원인**: 노드에 `NoExecute` Taint가 추가되었거나 노드 상태 이상으로 시스템 Taint가 자동 부여됨

**해결 방법**:
```bash
# 노드 이벤트 및 상태 확인
kubectl describe node <NODE_NAME>

# 노드에 자동 부여된 시스템 Taint 확인 (node.kubernetes.io/*)
kubectl get node <NODE_NAME> -o jsonpath='{.spec.taints}'

# 파드에 tolerationSeconds 추가하여 즉시 퇴출 방지
```

### 3.2 자주 발생하는 문제

#### Taint 오타로 인한 스케줄링 실패

**증상**: Toleration을 설정했는데도 파드가 Pending

**원인**: Taint의 key/value/effect와 Toleration의 값이 정확히 일치하지 않음 (대소문자, 오타)

**해결 방법**:
```bash
# 노드의 실제 Taint 값 확인
kubectl get node <NODE_NAME> -o jsonpath='{.spec.taints}' | python3 -m json.tool

# 파드의 Toleration 확인
kubectl get pod <POD_NAME> -n <NAMESPACE> -o jsonpath='{.spec.tolerations}'
```

#### 컨트롤플레인 노드에 파드가 배포되지 않음

**증상**: DaemonSet 파드가 컨트롤플레인 노드에만 누락됨

**원인**: 컨트롤플레인 노드의 기본 Taint `node-role.kubernetes.io/control-plane:NoSchedule`를 허용하지 않음

**해결 방법**:
```yaml
spec:
  tolerations:
    - key: "node-role.kubernetes.io/control-plane"
      operator: "Exists"
      effect: "NoSchedule"
```

---

## 4. 모니터링 및 확인

```bash
# 전체 노드의 Taint 현황 한눈에 확인
kubectl get nodes -o custom-columns=\
'NAME:.metadata.name,TAINTS:.spec.taints'

# 특정 노드 Taint 상세
kubectl describe node <NODE_NAME> | grep -A 10 "Taints:"

# 파드의 Toleration 확인
kubectl get pod <POD_NAME> -n <NAMESPACE> -o jsonpath='{.spec.tolerations}' | python3 -m json.tool

# 스케줄링 실패 파드 이벤트 확인
kubectl get events -n <NAMESPACE> --field-selector reason=FailedScheduling --sort-by='.lastTimestamp'

# 특정 파드가 어떤 노드에 스케줄링될 수 있는지 시뮬레이션
kubectl describe pod <POD_NAME> -n <NAMESPACE>
# Events 섹션의 "didn't match" 메시지 참조

# 노드별 Taint를 JSON으로 추출
kubectl get nodes -o json | \
  python3 -c "import sys,json; nodes=json.load(sys.stdin)['items']; \
  [print(n['metadata']['name'], n['spec'].get('taints','없음')) for n in nodes]"
```

---

## 5. TIP

- **Taint vs NodeAffinity 비교**:
  - Taint/Toleration: "특정 파드 **배제**" (기본 거부, 허용 선언)
  - NodeAffinity: "특정 노드에 **유인**" (기본 허용, 선호/강제 선택)
  - 두 기능을 조합하면 "특정 노드에만 특정 파드"를 완벽하게 구현 가능

- **스팟 인스턴스 활용 패턴**:
```bash
# 스팟 노드에 Taint 부여
kubectl taint nodes <SPOT_NODE> spot=true:NoSchedule

# 스팟 허용 파드에만 Toleration 추가 → 비용 최적화
```

- **kubectl taint 단축키**: `=` 없이 key만 쓰면 value 없는 Taint 생성
```bash
kubectl taint nodes <NODE_NAME> dedicated:NoSchedule
# key=dedicated, value 없음, effect=NoSchedule
```

- **자동 부여 시스템 Taint**: 노드 상태 이상 시 K8s가 자동으로 아래 Taint를 부여함
  - `node.kubernetes.io/not-ready:NoExecute`
  - `node.kubernetes.io/unreachable:NoExecute`
  - `node.kubernetes.io/memory-pressure:NoSchedule`
  - `node.kubernetes.io/disk-pressure:NoSchedule`
  - `node.kubernetes.io/pid-pressure:NoSchedule`
  - `node.kubernetes.io/unschedulable:NoSchedule`
