## 1. 개요 및 비유

파드 하나가 생성되고 삭제되기까지의 전 과정은 수십 개의 컴포넌트가 협력하는 **분산 이벤트 체인**입니다.

💡 **비유하자면 건물 입주/퇴거 프로세스와 같습니다.**
입주 신청(kubectl apply) → 심사(Scheduler) → 열쇠 전달(kubelet) → 입주 공사(컨테이너 런타임) → 점검(Probe) → 퇴거 통보(terminationGracePeriod) → 이사(컨테이너 종료) → 말소(etcd 삭제)

---

## 2. 파드 생성 전체 흐름

### 2.1 생성 시퀀스 (kubectl apply → 컨테이너 Running)

```
1. kubectl apply -f pod.yaml
        │
        ▼
2. API Server (인증 → 인가 → Admission → etcd 저장)
   파드 status.phase = Pending
   파드 spec.nodeName = "" (미배정)
        │
        ▼ Watch 이벤트 (ADDED)
3. Scheduler (kube-scheduler)
   - Filter 플러그인: 배치 불가 노드 제거
   - Score 플러그인: 최적 노드 점수 계산
   - Bind: spec.nodeName = "worker-2" 으로 업데이트
        │
        ▼ Watch 이벤트 (MODIFIED, nodeName 설정됨)
4. kubelet on worker-2
   - 자신의 nodeName과 일치하는 파드 감지
   - Pod Admission 검사 (리소스 여유, taint 등)
        │
        ▼
5. kubelet → containerd (CRI gRPC)
   - pause 컨테이너 생성 (NET/IPC/UTS 네임스페이스 소유)
   - CNI 플러그인 호출 → 네트워크 설정
        │
        ▼
6. Init 컨테이너 순차 실행
   - 각 init 컨테이너가 exitCode=0 으로 끝나야 다음으로
   - 실패 시 restartPolicy에 따라 재시도
        │
        ▼
7. 메인 컨테이너들 동시 시작
   - postStart 훅 실행 (컨테이너 시작 직후, 비동기)
   - startupProbe 통과 후 → livenessProbe / readinessProbe 시작
        │
        ▼
8. readinessProbe 성공 시
   - Endpoints 오브젝트에 파드 IP 추가
   - 트래픽 수신 시작
   status.phase = Running
```

### 2.2 kubelet의 Reconcile Loop 상세

kubelet은 루프를 돌면서 "원하는 상태"와 "실제 상태"를 지속적으로 맞춥니다.

```
kubelet PLEG (Pod Lifecycle Event Generator)
┌──────────────────────────────────────────────────┐
│ 매 1초마다:                                       │
│   실제 컨테이너 상태 조회 (containerd gRPC)       │
│   이전 상태와 비교 → 이벤트 생성                  │
│   ContainerStarted / ContainerDied / ...         │
└──────────────┬───────────────────────────────────┘
               │
               ▼
kubelet SyncPod (이벤트 발생 시):
  1. 파드 스펙과 실제 상태 비교
  2. 불필요한 컨테이너 제거
  3. 필요한 컨테이너 생성/재시작
  4. 볼륨 마운트 상태 동기화
  5. status 업데이트 → API Server에 반영
```

```bash
# kubelet이 관리하는 파드 상태 직접 확인
# kubelet의 /pods API 엔드포인트
curl -sk https://localhost:10250/pods \
  --cacert /var/lib/kubelet/pki/kubelet.crt \
  | jq '.items[].metadata.name'

# PLEG 이벤트 확인
journalctl -u kubelet | grep "PLEG"

# kubelet의 파드 동기화 로그
journalctl -u kubelet | grep "SyncPod"
```

---

## 3. 프로브(Probe) 심층 동작

### 3.1 세 가지 프로브의 역할 분리

```
startupProbe   : 앱이 아직 '기동 중'인가? (느린 앱 보호)
                 통과 전까지 liveness/readiness 검사 안 함
                 실패 시 → 컨테이너 재시작

livenessProbe  : 앱이 '살아있는가'? (데드락 감지)
                 실패 시 → 컨테이너 재시작 (파드 유지)

readinessProbe : 앱이 '트래픽 받을 준비가 됐는가'?
                 실패 시 → Endpoints에서 제거 (파드 유지, 트래픽만 차단)
```

### 3.2 프로브 실패와 재시작 정책

```
컨테이너 재시작 결정 트리:
livenessProbe 실패 (failureThreshold 초과)
        │
        ▼
컨테이너 강제 종료 (SIGKILL)
        │
        ▼
restartPolicy 확인
  Always   → 즉시 재시작 (backoff: 10s → 20s → 40s → 최대 5분)
  OnFailure→ exitCode != 0 이면 재시작
  Never    → 재시작 안 함

CrashLoopBackOff = 재시작 backoff 대기 중인 상태
```

```yaml
spec:
  containers:
  - name: app
    startupProbe:
      httpGet:
        path: /healthz
        port: 8080
      failureThreshold: 30   # 30회 * 10초 = 최대 300초 기동 허용
      periodSeconds: 10
    livenessProbe:
      httpGet:
        path: /healthz
        port: 8080
      initialDelaySeconds: 0  # startupProbe 통과 후 즉시
      periodSeconds: 10
      failureThreshold: 3
    readinessProbe:
      httpGet:
        path: /ready
        port: 8080
      periodSeconds: 5
      successThreshold: 1
      failureThreshold: 2
```

---

## 4. 파드 삭제 전체 흐름 (Graceful Shutdown)

```
kubectl delete pod my-pod
        │
        ▼
1. API Server: metadata.deletionTimestamp 설정
   (실제 삭제가 아닌 "삭제 예약" 표시)
        │
        ├──────────────────────────────────────┐
        ▼                                      ▼
2. Endpoint Controller:              2. kubelet:
   Endpoints에서 파드 IP 제거           terminationGracePeriodSeconds 타이머 시작
   (새 트래픽 차단 시작)                (기본 30초)
        │                                      │
        │                              3. preStop 훅 실행 (설정된 경우)
        │                                 sleep 또는 HTTP 콜백
        │                                      │
        │                              4. SIGTERM 신호 전송
        │                                 앱이 graceful shutdown 처리해야 함
        │                                 (커넥션 드레인, 작업 완료 등)
        │                                      │
        │                              5. 타이머 만료 시 SIGKILL 강제 종료
        │                                 (앱이 SIGTERM 무시했을 경우)
        │                                      │
        ▼                                      ▼
6. 컨테이너 종료 확인 → finalizer 처리 → etcd에서 파드 오브젝트 삭제
```

```yaml
# 충분한 graceful shutdown 시간 확보
spec:
  terminationGracePeriodSeconds: 60   # 기본 30초에서 증가
  containers:
  - name: app
    lifecycle:
      preStop:
        exec:
          command: ["/bin/sh", "-c", "sleep 5"]
          # preStop이 끝나야 SIGTERM 전송됨
          # sleep 5로 Endpoints 제거가 전파될 시간 확보
```

```bash
# 삭제 중인 파드 강제 종료 (finalizer 등으로 stuck된 경우)
kubectl delete pod my-pod --grace-period=0 --force

# 파드 종료 이벤트 확인
kubectl describe pod my-pod | grep -A5 "Events:"
kubectl get events --field-selector involvedObject.name=my-pod
```

---

## 5. 트러블슈팅

* **파드가 Pending에서 멈춤:**
  ```bash
  kubectl describe pod my-pod | grep -A10 "Events:"
  # Insufficient cpu/memory → 리소스 부족
  # no nodes are available → taint/affinity 문제
  # node(s) had untolerated taint → Toleration 추가 필요
  ```

* **CrashLoopBackOff:**
  ```bash
  # 이전 실행의 로그 확인 (--previous)
  kubectl logs my-pod --previous

  # 재시작 횟수와 마지막 종료 이유 확인
  kubectl get pod my-pod -o jsonpath='{.status.containerStatuses[0]}'
  # lastState.terminated.exitCode, reason 확인
  ```

* **Terminating에서 멈춤 (finalizer 문제):**
  ```bash
  # finalizer 확인
  kubectl get pod my-pod -o jsonpath='{.metadata.finalizers}'

  # finalizer 강제 제거 (컨트롤러가 죽은 경우)
  kubectl patch pod my-pod -p '{"metadata":{"finalizers":null}}'
  ```

* **readinessProbe 실패로 트래픽 못 받음:**
  ```bash
  # probe가 실제로 성공하는지 컨테이너 내부에서 직접 확인
  kubectl exec my-pod -- wget -qO- http://localhost:8080/ready
  # 또는
  kubectl exec my-pod -- curl -s http://localhost:8080/ready
  ```
