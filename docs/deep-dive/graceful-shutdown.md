## 1. 개요 및 비유

**Graceful Shutdown(그레이스풀 셧다운)**은 파드가 종료될 때 처리 중인 요청을 완료하고 새 연결을 거부한 뒤 안전하게 종료되는 과정입니다.

💡 **비유하자면 식당의 '영업 종료' 프로세스와 같습니다.**
문을 잠그기 전에(SIGTERM) 이미 앉아있는 손님(진행 중인 요청)은 식사를 마저 마칠 수 있게 기다리고, 새 손님은 받지 않습니다. 다 나가면(graceful period 이후) 불을 끕니다(SIGKILL).

---

## 2. 핵심 설명

### 파드 종료 시퀀스 (전체 흐름)
```
kubectl delete pod / Rolling Update 트리거
        ↓
1. Pod status → Terminating
   K8s가 모든 Endpoint(Service, NLB 타깃)에서 파드 IP 제거
        ↓
2. preStop Hook 실행 (정의된 경우)
   - exec: 명령 실행
   - httpGet: HTTP 요청 전송
        ↓
3. SIGTERM 신호 전달 (컨테이너 프로세스에게)
        ↓
4. terminationGracePeriodSeconds 대기 (기본 30초)
   - 애플리케이션이 처리 중인 요청 완료 후 스스로 종료
        ↓
5. 시간 초과 시 SIGKILL 강제 종료
```

### 핵심 파라미터

| 파라미터 | 위치 | 기본값 | 설명 |
|---|---|---|---|
| `terminationGracePeriodSeconds` | pod spec | 30초 | SIGTERM → SIGKILL 사이 유예 시간 |
| `preStop` | container lifecycle | 없음 | SIGTERM 전에 실행할 훅 |
| `readinessProbe` | container | 없음 | 파드가 트래픽 받을 준비가 됐는지 판단 |

### Endpoint 제거 타이밍 문제

```
[이상적인 상황]
Endpoint 제거 → kube-proxy/NLB 반영 → SIGTERM → 요청 없음 → 안전 종료

[실제 발생하는 문제]
SIGTERM 도착 → 프로세스 종료 시작
                             ↓ (비동기로 느리게 전파됨)
                             Endpoint 제거 kube-proxy에 반영 중
                             → 아직 새 요청이 파드로 유입됨
                             → Connection Refused 에러 발생
```

**원인:** Endpoint 제거는 etcd → API Server → Endpoints Controller → kube-proxy 전파에 수 초가 걸립니다. 이 시간 동안 파드가 이미 종료되면 유입된 요청이 실패합니다.

---

## 3. YAML 적용 예시

### 권장 Graceful Shutdown 설정 (NLB/ALB 연동 시)
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0   # 새 파드 Ready 후 기존 파드 종료
      maxSurge: 1
  template:
    spec:
      terminationGracePeriodSeconds: 60   # SIGKILL 유예 시간 (요청 처리 시간보다 길게)
      containers:
      - name: app
        image: my-app:2.0
        lifecycle:
          preStop:
            exec:
              # Endpoint 제거 전파(약 5~10초)를 기다리는 sleep
              # 이 sleep 동안 새 요청을 계속 처리하다가
              # sleep이 끝나면 SIGTERM이 오고 애플리케이션이 graceful하게 종료
              command: ["/bin/sh", "-c", "sleep 10"]
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
          failureThreshold: 3     # 3번 실패 시 Endpoint에서 제거
          successThreshold: 1
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 15
          periodSeconds: 10
          failureThreshold: 3     # 3번 실패 시 컨테이너 재시작
```

### preStop + 애플리케이션 레벨 graceful shutdown (Go 예시)
```go
// 애플리케이션에서 SIGTERM을 받으면
// 1. 새 요청 거부 (서버 Shutdown)
// 2. 처리 중인 요청 완료 대기
// 3. 프로세스 종료

func main() {
    srv := &http.Server{Addr: ":8080", Handler: handler}

    // SIGTERM 수신 채널
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatal(err)
        }
    }()

    <-quit  // SIGTERM 대기
    log.Println("SIGTERM 수신, graceful shutdown 시작...")

    // 30초 안에 처리 중인 요청 완료
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := srv.Shutdown(ctx); err != nil {
        log.Fatal("강제 종료:", err)
    }
    log.Println("서버 정상 종료")
}
```

### Spring Boot graceful shutdown 설정
```yaml
# application.yml
server:
  shutdown: graceful    # SIGTERM 시 처리 중인 요청 완료 후 종료

spring:
  lifecycle:
    timeout-per-shutdown-phase: 30s   # 최대 대기 시간
```

---

## 4. 트러블 슈팅

### 문제 1: 파드 종료 시 Connection Refused / 502 에러 발생

**원인:** SIGTERM이 먼저 도착해 앱이 종료됐는데, Endpoint 제거가 kube-proxy/NLB에 아직 반영 안 됨

**해결:**
```yaml
lifecycle:
  preStop:
    exec:
      command: ["/bin/sh", "-c", "sleep 10"]
# preStop 실행 시간은 terminationGracePeriodSeconds에 포함됨
# terminationGracePeriodSeconds >= preStop sleep + 앱 종료 시간
terminationGracePeriodSeconds: 60
```

---

### 문제 2: 파드가 30초 후 강제 종료됨 (SIGKILL)

**원인:** `terminationGracePeriodSeconds`(기본 30초) 내에 프로세스가 종료되지 않음

**해결:**
```yaml
terminationGracePeriodSeconds: 120  # 실제 요청 처리 시간 + 여유 시간으로 설정

# 애플리케이션 코드에서도 반드시 SIGTERM 핸들러 구현
# (기본적으로 아무것도 안 하면 OS가 프로세스를 바로 종료할 수도 있음)
```

---

### 문제 3: Rolling Update 중 일시적으로 에러율 증가

**원인과 해결 흐름:**
```
원인 진단:
kubectl get events --sort-by='.lastTimestamp'
kubectl logs <종료된-파드> --previous

일반적인 원인:
1. preStop sleep 없이 즉시 SIGTERM → Endpoint 전파 지연 중 요청 유입
   해결: preStop sleep 추가 (5~15초)

2. readinessProbe 없음 → 새 파드가 Ready 전에 트래픽 받음
   해결: readinessProbe 추가

3. maxUnavailable > 0 → 기존 파드 먼저 종료 후 새 파드 대기
   해결: maxUnavailable: 0, maxSurge: 1
```

---

### preStop sleep 시간 산정 기준

```
권장 preStop sleep = Endpoint 전파 시간 + 여유
                   = (kube-proxy 반영 시간 + NLB 드레이닝) + 여유

일반적인 값:
- 소규모 클러스터 (노드 < 20): 5~10초
- 대규모 클러스터 (노드 100+): 15~30초
- NLB connection draining 설정값 이상으로 설정

확인 방법:
kubectl get svc <서비스명> -o jsonpath='{.metadata.annotations}'
# aws-load-balancer-controller: connection_draining_enabled, deregistration_delay
```

---

### Graceful Shutdown 체크리스트

- [ ] `terminationGracePeriodSeconds` ≥ preStop sleep + 앱 종료 시간
- [ ] `preStop` sleep으로 Endpoint 전파 지연 보완
- [ ] `readinessProbe` 설정으로 Ready 상태 정확히 판단
- [ ] 애플리케이션 코드에서 SIGTERM 핸들러 구현
- [ ] `maxUnavailable: 0`으로 새 파드 Ready 후 기존 파드 종료
- [ ] NLB/ALB connection draining 시간 확인 (AWS 설정)
