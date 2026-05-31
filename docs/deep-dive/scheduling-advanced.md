## 1. 개요 및 비유
기본 스케줄러는 CPU/메모리만 보고 파드를 배치하지만, 실무에서는 훨씬 세밀한 제어가 필요합니다. **Taint/Toleration**, **Affinity**, **PriorityClass**, **Topology Spread Constraints**를 조합하면 파드의 배치를 정밀하게 설계할 수 있습니다.

💡 **비유하자면 '공연 좌석 배정 시스템'과 같습니다.**
VIP 전용석(Taint)에는 VIP 티켓(Toleration) 소지자만 앉을 수 있고, "친구 옆에 앉고 싶다(Affinity)"거나 "앞뒤로 몰리지 않게 골고루 앉아달라(Topology Spread)"는 요청도 처리합니다. 응급 환자(Critical PriorityClass)가 오면 일반 관객 자리를 비워주기도 합니다.

## 2. 핵심 설명

### 1) Taint & Toleration — 노드 접근 제한

**Taint**는 노드에 붙이는 "기피 표시"입니다. 해당 Taint를 **Toleration**으로 용인하는 파드만 해당 노드에 배치됩니다.

**Taint Effect 3가지:**

| Effect | 동작 |
|---|---|
| `NoSchedule` | Toleration 없는 파드는 스케줄 안 됨. 이미 실행 중인 파드는 유지 |
| `PreferNoSchedule` | Toleration 없어도 스케줄 가능하지만 최대한 기피 |
| `NoExecute` | 스케줄 안 되고, 이미 실행 중인 파드도 퇴거(Evict)됨 |

### 2) Node/Pod Affinity — 선호/기피 설정

- `requiredDuringSchedulingIgnoredDuringExecution`: **반드시** 조건 충족 (하드 룰)
- `preferredDuringSchedulingIgnoredDuringExecution`: **가급적** 조건 충족 (소프트 룰)

### 3) Topology Spread Constraints — 균등 분산

파드를 AZ, 노드 등의 토폴로지 단위로 균등하게 분산합니다. 한 AZ에 파드가 몰려서 AZ 장애 시 서비스 전체가 다운되는 것을 방지합니다.

### 4) PriorityClass — 우선순위 기반 선점

리소스가 부족할 때 낮은 우선순위 파드를 퇴거시키고 높은 우선순위 파드를 먼저 배치합니다.

## 3. YAML 적용 예시

### Taint & Toleration (GPU 노드 전용 설정)
```bash
# GPU 노드에 Taint 설정
kubectl taint nodes gpu-node-1 dedicated=gpu:NoSchedule
```

```yaml
# GPU 파드 — Toleration으로 GPU 노드에 배치 허용
spec:
  tolerations:
  - key: "dedicated"
    operator: "Equal"
    value: "gpu"
    effect: "NoSchedule"
  nodeSelector:
    dedicated: gpu   # Taint 용인 + nodeSelector 조합으로 GPU 노드만 선택
  containers:
  - name: training
    image: tensorflow/tensorflow:latest-gpu
    resources:
      limits:
        nvidia.com/gpu: 1
```

### Node Affinity (특정 AZ, 인스턴스 타입 지정)
```yaml
spec:
  affinity:
    nodeAffinity:
      # 반드시 충족: us-east-1a 또는 us-east-1b AZ에만 배치
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: topology.kubernetes.io/zone
            operator: In
            values: ["us-east-1a", "us-east-1b"]
      # 가급적 충족: memory-optimized 인스턴스 선호
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 80
        preference:
          matchExpressions:
          - key: node.kubernetes.io/instance-type
            operator: In
            values: ["r6i.2xlarge", "r6i.4xlarge"]
```

### Pod Affinity & Anti-Affinity (함께/따로 배치)
```yaml
spec:
  affinity:
    # 같은 AZ에 cache 파드가 있는 노드에 배치 (레이턴시 감소)
    podAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchLabels:
              app: redis-cache
          topologyKey: topology.kubernetes.io/zone

    # 같은 노드에 동일 앱 파드가 없도록 분산 (HA)
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchLabels:
            app: web
        topologyKey: kubernetes.io/hostname   # 노드 단위로 분산
```

### Topology Spread Constraints (AZ 균등 분산)
```yaml
spec:
  topologySpreadConstraints:
  - maxSkew: 1                          # AZ 간 파드 수 차이를 최대 1로 제한
    topologyKey: topology.kubernetes.io/zone
    whenUnsatisfiable: DoNotSchedule    # 조건 충족 불가 시 스케줄 거부
    labelSelector:
      matchLabels:
        app: web
  - maxSkew: 1
    topologyKey: kubernetes.io/hostname # 노드 단위로도 균등 분산
    whenUnsatisfiable: ScheduleAnyway   # 조건 충족 불가 시에도 최선을 다해 배치
    labelSelector:
      matchLabels:
        app: web
```

### PriorityClass (시스템 중요도에 따른 우선순위)
```yaml
# 높은 우선순위 클래스 정의
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: high-priority
value: 1000000             # 높을수록 먼저 스케줄됨
globalDefault: false
preemptionPolicy: PreemptLowerPriority  # 하위 파드 선점 허용
description: "중요 서비스용 높은 우선순위"

---
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: low-priority
value: 100
preemptionPolicy: Never    # 다른 파드를 선점하지 않음

---
# 파드에서 PriorityClass 적용
spec:
  priorityClassName: high-priority
  containers:
  - name: critical-app
    image: critical-app:1.0
```

## 4. 트러블 슈팅

* **파드가 특정 노드에만 몰려서 배치됨:**
  * `podAntiAffinity`와 `topologySpreadConstraints`를 함께 쓰세요. AntiAffinity는 "같은 노드에 두 개 이상 금지"를, Topology Spread는 "AZ 간 균등 분산"을 담당합니다.

* **Taint를 걸었는데 기존 파드가 퇴거되지 않음:**
  * `NoSchedule` 효과는 새 파드에만 적용됩니다. 기존 파드까지 퇴거하려면 `NoExecute`를 사용하세요.

* **PriorityClass 적용 후 중요하지 않은 파드가 갑자기 퇴거됨:**
  * `preemptionPolicy: Never`로 설정하면 해당 PriorityClass 파드는 낮은 우선순위 파드를 선점하지 않습니다. 중요도는 있지만 공격적인 선점을 원하지 않을 때 사용합니다.

* **Topology Spread가 작동하지 않고 파드가 Pending:**
  * `whenUnsatisfiable: DoNotSchedule`로 설정했는데 가용 노드가 없어서 `maxSkew` 조건을 만족할 수 없는 상황입니다. `ScheduleAnyway`로 바꾸거나 노드를 추가하세요.
  * `kubectl describe pod <파드명>` 이벤트에서 `didn't match pod topology spread constraints` 메시지로 확인 가능합니다.

* **스케줄링 결정 이유를 상세히 보고 싶을 때:**
  ```bash
  # 파드가 각 노드에 스케줄될 때 점수 확인
  kubectl get events --field-selector reason=FailedScheduling

  # 스케줄러 로그 확인
  kubectl logs -n kube-system -l component=kube-scheduler --tail=50
  ```
