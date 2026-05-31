## 1. 개요 및 비유

Kubernetes 스케줄러는 단순히 "빈 노드에 파드를 배치"하는 것이 아니라, **플러그인 프레임워크 기반의 파이프라인**으로 최적의 노드를 선택합니다.

💡 **비유하자면 AI 부동산 중개 시스템과 같습니다.**
수백 개 매물(노드) 중 조건 미달 물건을 필터링(Filter)하고, 남은 물건들에 점수를 매겨(Score) 가장 좋은 곳에 계약(Bind)합니다.

---

## 2. 스케줄링 파이프라인 전체 구조

```
Scheduling Cycle (동기, 순차)     Binding Cycle (비동기)
┌─────────────────────────────┐   ┌───────────────────┐
│                             │   │                   │
│  PreFilter                  │   │  PreBind          │
│  (파드 사전 처리, 캐시)       │   │  (볼륨 bind 등)   │
│         │                   │   │         │         │
│         ▼                   │   │         ▼         │
│  Filter (Filtering 단계)    │   │  Bind             │
│  불가 노드 제거 (병렬 실행)  │   │  (nodeName 업데이트)│
│         │                   │   │         │         │
│         ▼                   │   │         ▼         │
│  PostFilter                 │   │  PostBind         │
│  (필터 후 처리, 선점 등)     │   │  (완료 후 처리)   │
│         │                   │   │                   │
│         ▼                   │   └───────────────────┘
│  PreScore                   │
│  (점수 계산 사전 처리)        │
│         │                   │
│         ▼                   │
│  Score (Scoring 단계)       │
│  각 노드에 점수 부여 (병렬)  │
│         │                   │
│         ▼                   │
│  NormalizeScore             │
│  (0~100 정규화)              │
│         │                   │
│         ▼                   │
│  Reserve                    │
│  (선택된 노드 리소스 예약)    │
│         │                   │
│         ▼                   │
│  Permit                     │
│  (승인/대기/거부)            │
└─────────────────────────────┘
```

---

## 3. Filter 플러그인 상세

### 3.1 주요 Filter 플러그인

| 플러그인 | 설명 | 탈락 조건 |
|---|---|---|
| `NodeUnschedulable` | 노드 스케줄 가능 여부 | `spec.unschedulable: true` 인 노드 |
| `NodeAffinity` | nodeAffinity required 조건 | requiredDuringScheduling 불충족 |
| `TaintToleration` | Taint 검사 | NoSchedule taint에 Toleration 없음 |
| `NodePorts` | 포트 충돌 | hostPort 이미 사용 중 |
| `NodeResourcesFit` | 리소스 확인 | requests > allocatable |
| `VolumeBinding` | 볼륨 바인딩 가능성 | PVC의 topology 제약 불충족 |
| `InterPodAffinity` | 파드 간 어피니티 | requiredDuringScheduling 불충족 |

```bash
# 스케줄러가 특정 파드를 어떤 이유로 거부했는지 확인
kubectl describe pod my-pod | grep -A10 "Events:"
# 출력 예:
# 0/3 nodes are available:
#   1 node(s) had untolerated taint {key: value}
#   2 node(s) didn't match Pod's node affinity/selector

# 스케줄러 상세 로그 (verbosity 높임)
kubectl logs -n kube-system kube-scheduler-master -v=4 | grep "my-pod"
```

### 3.2 리소스 요청과 실제 할당 가능량

```
노드의 Allocatable 계산:
Allocatable = Node Capacity - kube-reserved - system-reserved - eviction-threshold

예:
Node Capacity:     CPU 4코어, Memory 16Gi
kube-reserved:     CPU 100m, Memory 340Mi  (kubelet, containerd용)
system-reserved:   CPU 100m, Memory 200Mi  (OS 프로세스용)
eviction-threshold:                Memory 100Mi (최소 여유분)
───────────────────────────────────────────────
Allocatable:       CPU 3800m, Memory 15Gi 정도

# 실제 Allocatable 확인
kubectl describe node worker-1 | grep -A5 "Allocatable:"
kubectl describe node worker-1 | grep -A10 "Allocated resources:"
```

---

## 4. Score 플러그인 상세

### 4.1 주요 Score 플러그인과 기본 가중치

| 플러그인 | 기본 가중치 | 점수 원칙 |
|---|---|---|
| `NodeResourcesBalancedAllocation` | 1 | CPU/메모리 사용률 균형이 좋을수록 높은 점수 |
| `NodeResourcesFit` (LeastAllocated) | 1 | 남은 리소스가 많을수록 높은 점수 |
| `InterPodAffinity` | 2 | preferred 어피니티 만족할수록 높은 점수 |
| `NodeAffinity` | 2 | preferred 어피니티 만족할수록 높은 점수 |
| `TaintToleration` | 1 | PreferNoSchedule taint 없을수록 높은 점수 |
| `ImageLocality` | 1 | 이미 이미지 있는 노드에 높은 점수 |

```
최종 노드 점수 계산:
finalScore(node) = Σ (plugin_score × plugin_weight) / Σ weights

동점 처리:
여러 노드가 동점 시 → 무작위로 하나 선택 (round-robin 방식)
→ 이 덕분에 자연스러운 파드 분산이 이루어짐
```

### 4.2 LeastAllocated vs MostAllocated

```yaml
# 기본: LeastAllocated (리소스 많이 남은 노드 선호)
# → 파드를 여러 노드에 분산 (HA 유리)

# MostAllocated (리소스 꽉 찬 노드 선호)
# → 노드를 최대한 채운 후 다음 노드 사용 (비용 절감, 노드 수 최소화)
apiVersion: kubescheduler.config.k8s.io/v1
kind: KubeSchedulerConfiguration
profiles:
- schedulerName: default-scheduler
  pluginConfig:
  - name: NodeResourcesFit
    args:
      scoringStrategy:
        type: MostAllocated   # 또는 LeastAllocated (기본)
        resources:
        - name: cpu
          weight: 1
        - name: memory
          weight: 1
```

---

## 5. 스케줄러 확장 — 커스텀 스케줄러

### 5.1 스케줄링 프레임워크 플러그인 직접 구현 (Go)

```go
// Filter 플러그인 예시: GPU 메모리 체크
type GPUMemoryFilter struct{}

func (f *GPUMemoryFilter) Name() string {
    return "GPUMemoryFilter"
}

func (f *GPUMemoryFilter) Filter(
    ctx context.Context,
    state *framework.CycleState,
    pod *v1.Pod,
    nodeInfo *framework.NodeInfo,
) *framework.Status {
    // 파드에 GPU 요청이 없으면 통과
    requestedGPU := pod.Spec.Containers[0].Resources.Requests["nvidia.com/gpu-memory"]
    if requestedGPU.IsZero() {
        return framework.NewStatus(framework.Success)
    }

    // 노드에 GPU 메모리가 충분한지 확인
    allocatableGPU := nodeInfo.Node().Status.Allocatable["nvidia.com/gpu-memory"]
    if requestedGPU.Cmp(allocatableGPU) > 0 {
        return framework.NewStatus(framework.Unschedulable,
            fmt.Sprintf("GPU memory insufficient: requested %v, available %v",
                requestedGPU, allocatableGPU))
    }
    return framework.NewStatus(framework.Success)
}
```

### 5.2 다중 스케줄러 운영

```yaml
# 커스텀 스케줄러 배포
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gpu-scheduler
  namespace: kube-system
spec:
  template:
    spec:
      containers:
      - name: gpu-scheduler
        image: my-gpu-scheduler:1.0
        args:
        - --config=/etc/scheduler/config.yaml
        - --leader-elect=true

---
# 파드에서 커스텀 스케줄러 지정
apiVersion: v1
kind: Pod
spec:
  schedulerName: gpu-scheduler   # 기본값: default-scheduler
  containers:
  - name: gpu-app
    resources:
      limits:
        nvidia.com/gpu: 1
```

---

## 6. 선점(Preemption) 메커니즘

```
높은 우선순위 파드가 스케줄링 안 될 때:
┌──────────────────────────────────────────────────────┐
│ 1. Filter 실패 → PostFilter 플러그인 실행            │
│                                                      │
│ 2. DefaultPreemption 플러그인:                       │
│    - 어떤 노드에서 어떤 파드를 제거하면 스케줄 가능한지 │
│      계산                                            │
│    - 제거할 파드의 우선순위가 높은 파드보다 낮아야 함  │
│                                                      │
│ 3. 선점 대상 노드 선택:                               │
│    - 가장 적은 수의 파드 제거로 스케줄 가능한 노드    │
│    - 제거되는 파드의 우선순위 합이 가장 낮은 노드     │
│                                                      │
│ 4. 선점될 파드들에 nominatedNodeName 표시            │
│    해당 파드들에 graceful termination 신호           │
│                                                      │
│ 5. 파드 종료 완료 후 → 높은 우선순위 파드 배치       │
└──────────────────────────────────────────────────────┘
```

```yaml
# PriorityClass 정의
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: high-priority
value: 1000000       # 높을수록 우선순위 높음
globalDefault: false
preemptionPolicy: PreemptLowerPriority  # 기본값
description: "중요 서비스용 높은 우선순위"

---
# 파드에 적용
spec:
  priorityClassName: high-priority
```

```bash
# 스케줄러의 선점 결정 확인
kubectl describe pod high-priority-pod | grep "NominatedNodeName"
# 아직 배치 못 됐지만 선점 예정 노드 표시됨

# 선점될 파드 확인
kubectl get events | grep Preempted
```

---

## 7. 트러블슈팅

* **파드가 특정 노드에만 배치됨 (분산이 안 됨):**
  ```bash
  # ImageLocality 때문일 수 있음 (이미지 이미 있는 노드 선호)
  # 해결: 모든 노드에 이미지 pre-pull

  # topologySpreadConstraints로 강제 분산
  spec:
    topologySpreadConstraints:
    - maxSkew: 1
      topologyKey: kubernetes.io/hostname
      whenUnsatisfiable: DoNotSchedule
      labelSelector:
        matchLabels:
          app: my-app
  ```

* **높은 우선순위 파드가 선점을 안 함:**
  ```bash
  # PriorityClass 확인
  kubectl get priorityclass
  kubectl describe priorityclass high-priority

  # 선점 정책 확인 (Never면 선점 안 함)
  kubectl get priorityclass high-priority -o jsonpath='{.preemptionPolicy}'

  # 스케줄러 로그에서 선점 계산 과정 확인
  kubectl logs -n kube-system kube-scheduler-master | grep -i preempt
  ```
