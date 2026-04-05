## 1. 개요 및 비유

노드의 메모리가 부족해지면 두 가지 방어 기제가 작동합니다. **Linux OOM Killer**는 커널 수준에서 프로세스를 강제 종료하고, **kubelet Eviction Manager**는 쿠버네티스 수준에서 파드를 점진적으로 퇴거시킵니다.

💡 **비유하자면 건물 화재 대피와 같습니다.**
Eviction은 화재 경보(압박 감지) 후 자발적 대피(파드 우선순위에 따라 퇴거). OOM Killer는 탈출 시간이 없을 때 건물이 강제로 특정 사람을 밀어내는 것(프로세스 강제 종료)입니다.

---

## 2. QoS 클래스와 우선순위

파드의 리소스 요청 방식에 따라 자동으로 QoS 클래스가 결정됩니다.

### 2.1 QoS 클래스 결정 기준

```
Guaranteed (최고 품질):
  - 모든 컨테이너의 requests == limits (CPU, Memory 모두)
  - OOM Killer에 의해 마지막으로 종료됨
  - Eviction 시 마지막 대상

Burstable (중간 품질):
  - 최소 하나의 컨테이너에 requests 또는 limits 설정
  - Guaranteed 조건 미충족

BestEffort (최하 품질):
  - 모든 컨테이너에 requests와 limits 모두 미설정
  - OOM Killer에 의해 가장 먼저 종료됨
  - Eviction 시 첫 번째 대상
```

```yaml
# Guaranteed 예시 (requests == limits)
spec:
  containers:
  - name: app
    resources:
      requests:
        cpu: "500m"
        memory: "256Mi"
      limits:
        cpu: "500m"    # requests와 동일
        memory: "256Mi" # requests와 동일

# BestEffort 예시 (아무 설정도 없음)
spec:
  containers:
  - name: app
    image: my-app:1.0
    # resources 섹션 없음
```

```bash
# 파드의 QoS 클래스 확인
kubectl get pod my-pod -o jsonpath='{.status.qosClass}'
# Guaranteed / Burstable / BestEffort

# 노드 내 파드별 QoS 확인
kubectl get pods -o custom-columns=\
NAME:.metadata.name,QoS:.status.qosClass
```

---

## 3. Linux OOM Killer 동작

### 3.1 OOM Score — 어떤 프로세스가 먼저 죽는가

```
OOM Score 계산:
oom_score_adj 범위: -1000 ~ +1000
  -1000: 절대 죽이지 않음 (kubelet, containerd 등)
  0: 기본값 (일반 프로세스)
  +1000: 가장 먼저 죽임

Kubernetes가 설정하는 oom_score_adj:
  Guaranteed 컨테이너:  -998 (거의 안 죽임)
  Burstable 컨테이너:    메모리 requests 비율에 따라 2~999
  BestEffort 컨테이너:  +1000 (가장 먼저 죽임)
  kubelet 프로세스:     -999 (절대 안 죽임)
  pause 컨테이너:       -998
```

```bash
# 컨테이너 프로세스의 OOM Score 확인
PID=$(crictl inspect <컨테이너ID> | jq '.info.pid')
cat /proc/$PID/oom_score_adj
# Guaranteed: -998
# BestEffort: 1000

# OOM 이벤트 로그 확인 (커널 로그)
dmesg | grep -i "oom\|out of memory\|killed process"
# 출력 예:
# [123456.789] Out of memory: Kill process 12345 (my-app) score 800 or sacrifice child
# [123456.790] Killed process 12345 (my-app), total-vm:524288kB, anon-rss:491520kB

# kubectl로 OOM Killed 파드 확인
kubectl get pods | grep OOMKilled
kubectl describe pod <pod-name> | grep -A3 "Last State:"
# Last State: Terminated
#   Reason: OOMKilled
#   Exit Code: 137 (128 + SIGKILL)
```

### 3.2 메모리 Limit 초과 시 흐름

```
컨테이너 메모리 사용량 > limits.memory
        │
        ▼
cgroup memory.max 초과
        │
        ▼
커널 OOM Killer 실행 (컨테이너 네임스페이스 내)
        │
        ▼
컨테이너 내 프로세스 강제 종료 (SIGKILL, exit 137)
        │
        ▼
containerd shim이 감지 → kubelet에 통보
        │
        ▼
kubelet: restartPolicy에 따라 재시작
파드 status: OOMKilled, restartCount 증가
CrashLoopBackOff로 진입 (반복 시)
```

---

## 4. kubelet Eviction Manager

### 4.1 Eviction 임계값 설정

```yaml
# /var/lib/kubelet/config.yaml
evictionHard:
  memory.available: "200Mi"   # 가용 메모리가 200Mi 이하면 즉시 퇴거
  nodefs.available: "10%"     # 노드 파일시스템 여유가 10% 이하
  nodefs.inodesFree: "5%"     # inode 여유가 5% 이하
  imagefs.available: "15%"    # 이미지 파일시스템 여유가 15% 이하

evictionSoft:
  memory.available: "500Mi"   # 500Mi 이하 되면 소프트 퇴거 시작
  nodefs.available: "15%"

evictionSoftGracePeriod:
  memory.available: "1m30s"   # 소프트 임계값 1분30초 지속 시 퇴거

evictionMinimumReclaim:
  memory.available: "100Mi"   # 퇴거 후 최소 100Mi 확보 후 중단
```

### 4.2 Eviction 대상 선정 알고리즘

```
노드 메모리 압박 발생
        │
        ▼
퇴거 대상 파드 선정 순서:
1. BestEffort 파드 (QoS 기준)
2. Burstable 파드 중 limits 초과한 것
3. Burstable 파드 (requests 대비 사용량 높은 순)
4. Guaranteed 파드 (limits 초과한 경우만)

동일 QoS 내에서 추가 정렬 기준:
- Pod Priority (priorityClassName)
- 메모리 requests 대비 실제 사용량 비율 (높을수록 먼저)
```

```bash
# 노드 압박 상태 확인
kubectl describe node worker-1 | grep -A5 "Conditions:"
# MemoryPressure: True → 메모리 Eviction 발생 중
# DiskPressure: True → 디스크 Eviction 발생 중
# PIDPressure: True → PID 부족

# Eviction 이벤트 확인
kubectl get events --field-selector reason=Evicted
kubectl get events -A | grep Evicted

# Evicted된 파드 목록
kubectl get pods -A | grep Evicted

# Evicted 파드 일괄 정리
kubectl get pods -A | grep Evicted | awk '{print $1 " " $2}' | \
  xargs -L1 -I{} sh -c 'kubectl delete pod -n {}'
```

### 4.3 Node Pressure Taint 자동 설정

```
노드 압박 발생 시 kubelet이 자동으로 Taint 추가:
MemoryPressure → node.kubernetes.io/memory-pressure:NoSchedule
DiskPressure   → node.kubernetes.io/disk-pressure:NoSchedule
PIDPressure    → node.kubernetes.io/pid-pressure:NoSchedule
NotReady       → node.kubernetes.io/not-ready:NoExecute

NoExecute Taint: 이미 실행 중인 파드도 퇴거!
→ Toleration 없는 파드는 즉시 퇴거
→ 기본 Toleration: notReady:NoExecute 300초 (tolerationSeconds)
```

---

## 5. YAML 적용 예시

### 프로덕션 서비스 안정성 보장 설정

```yaml
apiVersion: v1
kind: Pod
spec:
  # Guaranteed QoS로 OOM Killer 최후 대상 설정
  containers:
  - name: app
    resources:
      requests:
        cpu: "500m"
        memory: "512Mi"
      limits:
        cpu: "500m"      # requests와 동일 → Guaranteed
        memory: "512Mi"

  # OOM 발생 시 다른 파드에 영향 최소화
  priorityClassName: high-priority

---
# 중요 시스템 파드가 Eviction 안 되도록
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: my-app-pdb
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: my-app
```

---

## 6. 트러블슈팅

* **파드가 반복적으로 OOMKilled됨:**
  ```bash
  # 실제 메모리 사용량 확인 (limits 조정 필요한지)
  kubectl top pod my-pod --containers
  # Working Set Memory 기준으로 limits 설정

  # 메모리 사용 급증 패턴 확인
  kubectl top pod my-pod --containers -w   # watch 모드

  # limits를 실제 사용량의 1.5~2배로 설정 권장
  ```

* **노드가 NotReady 상태로 전환:**
  ```bash
  # 노드 압박 상태 확인
  kubectl describe node worker-1 | grep -E "Pressure|Condition"

  # 메모리 사용량 상위 파드 확인
  kubectl top pods -A --sort-by=memory | head -20

  # 디스크 사용량 확인 (imagefs)
  ssh worker-1 df -h
  ssh worker-1 crictl rmi --prune   # 이미지 정리
  ```

* **중요한 파드가 Eviction됨:**
  ```bash
  # Guaranteed QoS 확인
  kubectl get pod my-pod -o jsonpath='{.status.qosClass}'

  # PriorityClass 설정 확인
  kubectl get pod my-pod -o jsonpath='{.spec.priorityClassName}'

  # Eviction 방지: requests == limits 설정 + 높은 PriorityClass
  ```
