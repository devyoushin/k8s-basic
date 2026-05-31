## 1. 개요 및 비유
쿠버네티스에서 리소스 관리는 단순히 `requests`/`limits`를 설정하는 것 이상입니다. **QoS 클래스**, **VPA**, **Eviction** 메커니즘을 이해해야 클러스터에서 파드가 왜 죽는지, 어떤 파드가 먼저 쫓겨나는지 예측할 수 있습니다.

💡 **비유하자면 '비행기 좌석 등급 시스템'과 같습니다.**
비즈니스석(Guaranteed) 승객은 기상 악화가 심해도 좌석이 보장되지만, 얼리버드 이코노미(Burstable)는 초과 예약 시 조정될 수 있고, 대기 예약(BestEffort)은 자리가 남을 때만 탑승 가능합니다.

## 2. 핵심 설명

### QoS (Quality of Service) 클래스

Kubelet은 노드 메모리가 부족할 때 파드를 자동으로 퇴거(Evict)합니다. 어떤 파드를 먼저 퇴거할지는 **QoS 클래스**로 결정됩니다.

| QoS 클래스 | 조건 | 퇴거 우선순위 | 특징 |
|---|---|---|---|
| **Guaranteed** | 모든 컨테이너에 requests == limits 설정 | 마지막 (가장 안전) | OOM 시 가장 나중에 킬됨 |
| **Burstable** | 일부 컨테이너에 requests 또는 limits 설정 | 중간 | 대부분의 일반 파드 |
| **BestEffort** | requests도 limits도 없음 | 가장 먼저 | 노드 여유 자원 사용 |

```bash
# 파드의 QoS 클래스 확인
kubectl get pod <파드명> -o jsonpath='{.status.qosClass}'
```

### requests vs limits 동작 원리

```
requests: 스케줄러가 노드 배치 시 사용하는 "예약된 자원"
          → 이 값 기준으로 노드에 파드를 배치
          → HPA의 % 계산 기준

limits:   컨테이너가 실제로 사용할 수 있는 "최대 자원"
          → CPU: 초과 시 쓰로틀링 (죽지 않음)
          → Memory: 초과 시 OOMKilled (재시작됨)
```

### CPU vs Memory 제한 동작 차이

| 자원 | limits 초과 시 | 결과 |
|---|---|---|
| **CPU** | cgroup으로 쓰로틀링 | 느려짐, 죽지 않음 |
| **Memory** | 커널 OOM Killer 동작 | 프로세스 강제 종료 → 파드 재시작 |

### VPA (Vertical Pod Autoscaler)
HPA가 파드 수를 늘리는 것이라면, VPA는 파드의 requests/limits 값 자체를 자동으로 조정합니다.

| 모드 | 동작 |
|---|---|
| `Off` | 권장값만 계산 (적용 안 함) |
| `Initial` | 파드 생성 시에만 권장값 적용 |
| `Recreate` | 현재 값이 범위 벗어나면 파드 재시작하여 적용 |
| `Auto` | 권장값 자동 적용 (재시작 포함) |

> **HPA + VPA 동시 사용 주의:** 같은 메트릭(CPU/Memory)을 두 오토스케일러가 동시에 제어하면 충돌합니다. HPA는 커스텀 메트릭으로, VPA는 리소스 설정으로 역할을 분리하세요.

### Eviction 메커니즘
Kubelet이 노드 자원 부족을 감지하면 다음 순서로 파드를 퇴거합니다.

```
1. BestEffort 파드 (requests/limits 없음)
2. Burstable 파드 중 requests 초과 사용 중인 파드
3. Guaranteed 파드 (최후의 수단)
```

## 3. YAML 적용 예시

### Guaranteed QoS (프로덕션 핵심 서비스)
```yaml
spec:
  containers:
  - name: api
    image: my-api:1.0
    resources:
      requests:
        cpu: "500m"
        memory: "512Mi"
      limits:
        cpu: "500m"     # requests == limits → Guaranteed
        memory: "512Mi"
```

### Burstable QoS (일반적인 설정)
```yaml
spec:
  containers:
  - name: worker
    image: my-worker:1.0
    resources:
      requests:
        cpu: "100m"      # 평소에는 100m만 예약
        memory: "128Mi"
      limits:
        cpu: "1000m"     # 필요 시 최대 1코어까지 버스트
        memory: "512Mi"
```

### VPA 설정
```yaml
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: api-vpa
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: api
  updatePolicy:
    updateMode: "Auto"        # 자동 적용 (파드 재시작 포함)
  resourcePolicy:
    containerPolicies:
    - containerName: api
      minAllowed:
        cpu: "50m"
        memory: "64Mi"
      maxAllowed:
        cpu: "4"
        memory: "4Gi"
      controlledResources: ["cpu", "memory"]
```

```bash
# VPA 권장값 확인 (updateMode: Off 일 때 유용)
kubectl describe vpa api-vpa

# 출력 예시:
# Recommendation:
#   Container Recommendations:
#     Container Name: api
#       Lower Bound:   cpu: 80m, memory: 200Mi
#       Target:        cpu: 120m, memory: 300Mi  ← 이 값으로 설정 권장
#       Upper Bound:   cpu: 500m, memory: 1Gi
```

### Kubelet Eviction 임계값 설정
```yaml
# /var/lib/kubelet/config.yaml
apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
evictionHard:
  memory.available: "200Mi"    # 가용 메모리 200Mi 미만이면 즉시 퇴거 시작
  nodefs.available: "10%"      # 디스크 잔량 10% 미만
  nodefs.inodesFree: "5%"      # inode 잔량 5% 미만
evictionSoft:
  memory.available: "500Mi"    # 500Mi 미만이면 grace period 후 퇴거
evictionSoftGracePeriod:
  memory.available: "1m30s"    # 1분 30초 유예 기간
evictionMinimumReclaim:
  memory.available: "100Mi"    # 퇴거 후 최소 100Mi 회수
```

### LimitRange로 네임스페이스 기본값 강제
```yaml
# requests/limits 없는 파드에 자동으로 기본값 부여 → BestEffort 방지
apiVersion: v1
kind: LimitRange
metadata:
  name: default-resources
  namespace: production
spec:
  limits:
  - type: Container
    default:
      cpu: "500m"
      memory: "256Mi"
    defaultRequest:
      cpu: "100m"
      memory: "128Mi"
    max:
      cpu: "4"
      memory: "4Gi"
    min:
      cpu: "50m"
      memory: "64Mi"
```

## 4. 트러블 슈팅

* **파드가 `OOMKilled`로 계속 재시작됨:**
  ```bash
  # 이전 컨테이너의 종료 사유 확인
  kubectl describe pod <파드명> | grep -A5 "Last State"
  # Exit Code: 137 = OOMKilled

  # 실제 메모리 사용량 확인
  kubectl top pod <파드명> --containers
  ```
  * `limits.memory`를 올리거나, 앱의 메모리 누수를 디버깅하세요.
  * VPA `Off` 모드로 권장 메모리 값을 먼저 관찰한 후 설정하는 방법도 유효합니다.

* **CPU throttling이 심한데 limits를 올리기 부담스러울 때:**
  ```bash
  # CPU throttling 비율 확인 (Prometheus 메트릭)
  # container_cpu_cfs_throttled_seconds_total / container_cpu_cfs_periods_total
  ```
  * `limits.cpu`를 올리거나, requests와 limits의 비율을 줄여 예측 가능성을 높이세요.
  * Guaranteed QoS로 설정하면 쓰로틀링이 줄어드는 경우가 많습니다.

* **노드에 자원이 충분한데 파드가 퇴거됨:**
  * `evictionHard` 임계값에 걸린 것입니다. `kubectl describe node <노드명>` 에서 `Conditions` 섹션의 `MemoryPressure`, `DiskPressure` 를 확인하세요.

* **VPA가 파드를 너무 자주 재시작함:**
  * `updateMode: "Initial"`로 변경하면 파드 생성 시에만 값을 적용하고 실행 중에는 재시작하지 않습니다.
  * 또는 `minAllowed`와 `maxAllowed` 범위를 좁혀서 권장값 변동폭을 줄이세요.
