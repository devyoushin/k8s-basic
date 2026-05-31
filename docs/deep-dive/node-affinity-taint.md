## 1. 개요 및 비유
**Node Affinity & Taint/Toleration 심화**는 파드를 올바른 노드에 배치하기 위한 쿠버네티스의 정밀 스케줄링 메커니즘입니다. 기본 개념을 넘어 내부 동작 원리, 실무 패턴, 엣지 케이스를 다룹니다.

💡 **비유하자면 '호텔 객실 배정 시스템'과 같습니다.**
일부 방은 VIP 전용(Taint)이라 VIP 카드(Toleration)가 없으면 들어갈 수 없고, 손님은 "높은 층 선호(preferredAffinity)"하거나 "반드시 금연실(requiredAffinity)"을 요구할 수 있습니다. 같은 일행끼리 붙거나(PodAffinity), 경쟁자와 분리(PodAntiAffinity)하는 요청도 처리합니다.

---

## 2. 핵심 설명

### 1) Taint & Toleration 내부 동작

Taint는 노드에 설정하고, Toleration은 파드 스펙에 설정합니다. 스케줄러는 파드의 Toleration이 노드의 모든 Taint를 **커버(cover)** 하는지 확인합니다.

**Toleration 매칭 규칙:**

```
Taint:       key=dedicated, value=gpu, effect=NoSchedule
Toleration:  key=dedicated, operator=Equal, value=gpu, effect=NoSchedule → ✅ 매칭

Taint:       key=dedicated, value=gpu, effect=NoSchedule
Toleration:  key=dedicated, operator=Exists                               → ✅ 매칭 (value 무관)

Taint:       key=dedicated, value=gpu, effect=NoSchedule
Toleration:  key=*, operator=Exists, effect=""                            → ✅ 모든 Taint 허용 (와일드카드)
```

**NoExecute + tolerationSeconds (임시 배치):**
```
[노드에 NoExecute Taint 추가]
    │
    ├─ Toleration 없는 파드 → 즉시 퇴거(Evict)
    ├─ Toleration 있는 파드 → 계속 실행
    └─ tolerationSeconds: 300 → 300초 후 퇴거 (임시 허용)
```

**쿠버네티스가 자동으로 추가하는 시스템 Taint:**

| Taint | 조건 | 효과 |
|---|---|---|
| `node.kubernetes.io/not-ready` | 노드 NotReady | NoExecute |
| `node.kubernetes.io/unreachable` | 노드 연결 불가 | NoExecute |
| `node.kubernetes.io/memory-pressure` | 메모리 부족 | NoSchedule |
| `node.kubernetes.io/disk-pressure` | 디스크 부족 | NoSchedule |
| `node.kubernetes.io/unschedulable` | 스케줄 비활성화 | NoSchedule |

> 일반 파드는 기본적으로 `not-ready`와 `unreachable`에 대해 `tolerationSeconds: 300`이 자동 설정됩니다. DaemonSet 파드는 이 Taint들을 무제한 허용합니다.

### 2) Node Affinity — 스케줄링 규칙

**두 가지 타이밍:**

| 타이밍 | 설명 |
|---|---|
| `DuringScheduling` | 파드가 처음 노드에 배치될 때 적용 |
| `DuringExecution` | 이미 실행 중인 파드에도 적용 (노드 레이블 변경 시) |

**현재 지원하는 4가지 조합:**

| 타입 | 동작 |
|---|---|
| `requiredDuringSchedulingIgnoredDuringExecution` | 배치 시 필수 조건, 실행 중엔 무시 |
| `preferredDuringSchedulingIgnoredDuringExecution` | 배치 시 선호(가중치), 실행 중엔 무시 |
| `requiredDuringSchedulingRequiredDuringExecution` | 배치 + 실행 중 모두 필수 (베타 기능) |
| `preferredDuringSchedulingRequiredDuringExecution` | 배치 시 선호, 실행 중엔 필수 (베타 기능) |

**matchExpressions 연산자:**

| 연산자 | 의미 |
|---|---|
| `In` | 레이블 값이 지정 목록 중 하나 |
| `NotIn` | 레이블 값이 지정 목록에 없음 |
| `Exists` | 레이블 키가 존재 (값 무관) |
| `DoesNotExist` | 레이블 키가 없음 |
| `Gt` | 레이블 값이 지정 값보다 큼 (숫자) |
| `Lt` | 레이블 값이 지정 값보다 작음 (숫자) |

### 3) Affinity vs nodeSelector vs nodeName 비교

| 방법 | 유연성 | 권장 용도 |
|---|---|---|
| `nodeName` | 없음 | 특정 노드 강제 지정 (비권장) |
| `nodeSelector` | 낮음 | 단순 레이블 매칭 |
| `nodeAffinity` | 높음 | 복잡한 조건, OR/AND 논리, 가중치 |

### 4) Pod Affinity/AntiAffinity 심화

`topologyKey`는 파드 분산의 **단위**를 결정합니다:

| topologyKey | 분산 단위 |
|---|---|
| `kubernetes.io/hostname` | 노드 단위 |
| `topology.kubernetes.io/zone` | AZ 단위 |
| `topology.kubernetes.io/region` | 리전 단위 |
| `커스텀 레이블` | 사용자 정의 단위 |

**PodAffinity 성능 주의:** 클러스터가 크면 모든 파드-파드 관계를 계산하므로 스케줄링 지연이 발생할 수 있습니다. Namespace 범위를 `namespaceSelector`로 제한하세요.

---

## 3. YAML 적용 예시

### Taint & Toleration 실전 패턴

#### 패턴 1: GPU 노드 전용 격리
```bash
# GPU 노드에 Taint 설정
kubectl taint nodes gpu-node-1 gpu-node-2 dedicated=gpu:NoSchedule
```

```yaml
# GPU 워크로드
spec:
  tolerations:
  - key: "dedicated"
    operator: "Equal"
    value: "gpu"
    effect: "NoSchedule"
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: dedicated
            operator: In
            values: ["gpu"]    # Taint 허용 + Affinity로 GPU 노드 선호 지정
  containers:
  - name: training
    resources:
      limits:
        nvidia.com/gpu: 2
```

#### 패턴 2: NoExecute + tolerationSeconds (점진적 퇴거)
```yaml
# 유지보수 중 노드에서 5분 후 퇴거 허용
spec:
  tolerations:
  - key: "node.kubernetes.io/unschedulable"
    operator: "Exists"
    effect: "NoExecute"
    tolerationSeconds: 300    # 300초 동안은 유지, 이후 퇴거

  # 노드 장애 시 즉시 퇴거하지 않고 30초 대기 (네트워크 순단 허용)
  - key: "node.kubernetes.io/unreachable"
    operator: "Exists"
    effect: "NoExecute"
    tolerationSeconds: 30
```

#### 패턴 3: 마스터 노드에 파드 배치 허용 (특수 상황)
```yaml
# 마스터 노드에는 기본적으로 아래 Taint가 존재:
# node-role.kubernetes.io/control-plane:NoSchedule
spec:
  tolerations:
  - key: "node-role.kubernetes.io/control-plane"
    operator: "Exists"
    effect: "NoSchedule"
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: node-role.kubernetes.io/control-plane
            operator: Exists
```

### Node Affinity 복합 조건
```yaml
spec:
  affinity:
    nodeAffinity:
      # 필수 조건: OR 논리 (두 nodeSelectorTerms 중 하나 충족)
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        # 조건 세트 1: us-east-1 리전 + 메모리 최적화 인스턴스
        - matchExpressions:
          - key: topology.kubernetes.io/region
            operator: In
            values: ["us-east-1"]
          - key: node.kubernetes.io/instance-type
            operator: In
            values: ["r6i.2xlarge", "r6i.4xlarge", "r6i.8xlarge"]
        # 조건 세트 2: us-west-2 리전 + 특정 AZ (대안 배치 경로)
        - matchExpressions:
          - key: topology.kubernetes.io/region
            operator: In
            values: ["us-west-2"]
          - key: topology.kubernetes.io/zone
            operator: In
            values: ["us-west-2a", "us-west-2b"]

      # 선호 조건: 가중치 합산으로 최적 노드 선택
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 80           # 최고 가중치: SSD 노드 강력 선호
        preference:
          matchExpressions:
          - key: node-storage-type
            operator: In
            values: ["ssd", "nvme"]
      - weight: 40           # 중간 가중치: 스팟 인스턴스 기피
        preference:
          matchExpressions:
          - key: node-lifecycle
            operator: NotIn
            values: ["spot"]
      - weight: 20           # 낮은 가중치: 특정 커널 버전 선호
        preference:
          matchExpressions:
          - key: kernel-version
            operator: Gt
            values: ["5"]
```

### Pod AntiAffinity — HA 배포 패턴
```yaml
# 웹 서버: 노드 단위 분산(하드) + AZ 단위 분산(소프트)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-server
spec:
  replicas: 6
  template:
    spec:
      affinity:
        podAntiAffinity:
          # 하드 룰: 동일 노드에 같은 앱 파드 금지
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                app: web-server
            topologyKey: kubernetes.io/hostname
          # 소프트 룰: 다른 AZ에 분산 선호
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  app: web-server
              topologyKey: topology.kubernetes.io/zone
              # namespaceSelector로 범위 제한 (성능 최적화)
              namespaceSelector:
                matchLabels:
                  kubernetes.io/metadata.name: production
```

### NodeAffinity + Taint/Toleration 조합 전략
```yaml
# 올바른 조합: Taint는 "들어올 수 있는지", Affinity는 "여기로 오고 싶은지"
# 두 가지를 함께 써야 노드를 완전히 예약(reserve)할 수 있음
spec:
  # 1단계: GPU Taint 허용 (들어갈 자격 부여)
  tolerations:
  - key: "dedicated"
    operator: "Equal"
    value: "gpu"
    effect: "NoSchedule"

  # 2단계: GPU 노드만 선택 (자격 있어도 GPU 노드로만 유도)
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: dedicated
            operator: In
            values: ["gpu"]

  # Taint만 있으면: GPU 외 파드가 다른 노드에 배치되고, GPU 파드도 다른 노드에 배치 가능
  # Affinity만 있으면: GPU 파드는 GPU 노드를 원하지만 GPU 노드에 다른 파드도 배치 가능
  # 조합: GPU 파드만 GPU 노드에 배치 (완전 예약)
```

---

## 4. 트러블 슈팅

* **Taint가 있는데 파드가 배치됨:**
  * 파드 스펙에 모든 Taint를 허용하는 Toleration(`operator: Exists`, effect 비어있음)이 설정되어 있는지 확인합니다.
  * `kubectl describe pod <이름>` → Tolerations 섹션 확인.
  * DaemonSet 파드는 기본적으로 시스템 Taint를 허용하므로 정상입니다.

* **requiredAffinity 조건인데 파드가 Pending:**
  ```bash
  kubectl describe pod <이름>
  # Events 섹션에서 "didn't match node selector" 또는
  # "node(s) didn't match Pod's node affinity/selector" 확인

  # 현재 노드 레이블 목록 확인
  kubectl get nodes --show-labels

  # 특정 레이블을 가진 노드만 필터링
  kubectl get nodes -l topology.kubernetes.io/zone=us-east-1a
  ```

* **preferredAffinity가 전혀 반영 안 되는 것 같음:**
  * preferredAffinity는 보장이 아닌 힌트입니다. 스케줄러가 다른 요소(리소스 가용량, spread constraints)를 우선할 수 있습니다.
  * `weight` 값을 높여서 우선순위를 강화하거나, required로 변경하세요.

* **NoExecute Taint 후 파드가 즉시 퇴거되어 서비스 중단:**
  * 기본 `tolerationSeconds`가 없으면 즉시 퇴거됩니다. 중요 파드에는 `tolerationSeconds: 60` 이상을 설정해 그레이스풀 종료 시간을 확보하세요.
  * `PodDisruptionBudget(PDB)`과 함께 사용하면 퇴거 속도를 제어할 수 있습니다.

* **Pod AntiAffinity로 분산했는데 Replica 수 증가 후 Pending:**
  * `requiredAntiAffinity`는 노드 수보다 Replica가 많을 수 없습니다. 노드 수 ≥ Replica 수 관계를 유지하거나, `preferredAntiAffinity`로 완화하세요.

* **노드 레이블 실시간 확인 및 변경:**
  ```bash
  # 노드 레이블 추가
  kubectl label node worker-1 dedicated=gpu node-storage-type=ssd

  # 노드 레이블 제거
  kubectl label node worker-1 dedicated-

  # Taint 추가/제거
  kubectl taint nodes worker-1 dedicated=gpu:NoSchedule
  kubectl taint nodes worker-1 dedicated=gpu:NoSchedule-    # 끝에 -는 제거

  # 스케줄링 비활성화 (노드 유지보수)
  kubectl cordon worker-1      # NoSchedule Taint 추가와 유사
  kubectl drain worker-1 --ignore-daemonsets --delete-emptydir-data
  kubectl uncordon worker-1    # 유지보수 완료 후 복귀
  ```
