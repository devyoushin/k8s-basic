## 1. 개요 및 비유

eBPF 기반 관찰 가능성은 애플리케이션 코드 수정 없이 커널 수준에서 네트워크 흐름, 시스템 콜, 성능 데이터를 수집합니다.

💡 **비유하자면 건물 내부에 투명한 유리 파이프를 설치하는 것과 같습니다.**
배관(코드)을 뜯지 않고도 어디서 물(트래픽)이 흐르는지, 어느 파이프가 막혔는지(지연), 얼마나 흐르는지(처리량)를 실시간으로 볼 수 있습니다.

---

## 2. eBPF 관찰 가능성 도구 비교

```
도구           강점                     약점
─────────────────────────────────────────────────────────
Cilium Hubble  L3~L7 네트워크 플로우    Cilium 필수
               서비스 맵 자동 생성
               Prometheus 연동

Pixie          코드리스 프로파일링      클라우드 의존 (CE)
               HTTP/SQL/gRPC 자동 캡처 에이전트 오버헤드

Tetragon       보안 이벤트 특화         설정 복잡
(Cilium)       프로세스/파일 추적

Parca          CPU 프로파일링           네트워크 추적 없음
               Flame Graph 시각화

Coroot         인프라 + APM 통합        상대적으로 새 도구
               eBPF + 에이전트 없는 APM
```

---

## 3. Cilium Hubble — L7 네트워크 관찰

### 3.1 Hubble 아키텍처

```
Cilium (eBPF 기반 CNI)
    │ 모든 패킷 관찰
    ▼
Hubble Agent (각 노드 DaemonSet)
    │ 이벤트 스트리밍 (gRPC)
    ▼
Hubble Relay (중앙 집계)
    │
    ├── Hubble UI   (서비스 맵, 실시간 플로우)
    └── Hubble API  (CLI, Prometheus)

관찰 레이어:
- L3: IP 패킷 (출처/목적지 IP, 프로토콜)
- L4: TCP/UDP (포트, 상태, 지연)
- L7: HTTP/gRPC/DNS (URL, 메서드, 상태코드, 지연)
```

### 3.2 설치 및 사용

```bash
# Cilium + Hubble 설치
helm repo add cilium https://helm.cilium.io/
helm install cilium cilium/cilium \
  --namespace kube-system \
  --set hubble.enabled=true \
  --set hubble.relay.enabled=true \
  --set hubble.ui.enabled=true \
  --set hubble.metrics.enabled="{dns,drop,tcp,flow,icmp,http}"

# Hubble CLI 설치
export HUBBLE_VERSION=$(curl -s https://raw.githubusercontent.com/cilium/hubble/master/stable.txt)
curl -L --remote-name-all https://github.com/cilium/hubble/releases/download/$HUBBLE_VERSION/hubble-linux-amd64.tar.gz
tar xzvf hubble-linux-amd64.tar.gz
sudo mv hubble /usr/local/bin

# Hubble Relay에 포트포워딩
kubectl port-forward -n kube-system svc/hubble-relay 4245:80 &

# 실시간 네트워크 플로우 관찰
hubble observe --follow

# HTTP 트래픽만 필터
hubble observe --protocol http --follow

# 특정 네임스페이스 트래픽
hubble observe --namespace production --follow

# DROP 이벤트 (NetworkPolicy 차단)
hubble observe --verdict DROPPED --follow
```

### 3.3 Hubble UI — 서비스 맵

```bash
# Hubble UI 포트포워딩
kubectl port-forward -n kube-system svc/hubble-ui 12000:80

# 브라우저에서 http://localhost:12000 접속
# → 클러스터 내 서비스 간 트래픽 자동 시각화
# → 각 경로의 요청 수, 지연, 에러율 표시
```

---

## 4. Hubble 메트릭으로 L7 관찰

```bash
# DNS 쿼리 실패 확인
hubble observe --protocol dns --verdict DROPPED --follow
# → 어떤 파드에서 어떤 도메인 쿼리가 실패하는지 즉시 확인

# HTTP 에러 확인 (4xx, 5xx)
hubble observe --protocol http \
  --http-status "4+" \
  --follow

# 특정 서비스 간 지연 확인
hubble observe \
  --from-label app=frontend \
  --to-label app=backend \
  --protocol http \
  --output json | jq '.flow.l7.http.latency_ns'

# Prometheus 메트릭 조회
kubectl port-forward -n kube-system svc/hubble-metrics 9091:9091 &
curl http://localhost:9091/metrics | grep hubble_http
# hubble_http_requests_total{method="GET",protocol="HTTP/1.1",reporter="server",status="200"} 1234
# hubble_http_request_duration_seconds{...} ...
```

---

## 5. Pixie — 코드리스 APM

### 5.1 Pixie 특징

```
Pixie가 자동으로 캡처하는 데이터 (에이전트/코드 수정 없음):
- HTTP/1.x, HTTP/2 요청/응답 (URL, 메서드, 상태코드, 지연)
- gRPC 호출 (서비스명, 메서드명, 에러)
- MySQL/PostgreSQL 쿼리 (SQL 내용, 실행 시간)
- Redis 명령
- Kafka 메시지
- DNS 쿼리
- JVM 힙, GC 통계 (Java 앱)
- Go/C++ 함수 프로파일링
```

```bash
# Pixie CLI 설치
bash -c "$(curl -fsSL https://withpixie.ai/install.sh)"

# Pixie 클러스터 배포
px deploy

# 실시간 HTTP 요청 확인 (PxL 스크립트)
px run px/http_data

# 특정 네임스페이스의 서비스 맵
px run px/service_stats -- -start_time:"-5m" -namespace:production

# SQL 쿼리 분석
px run px/mysql_data
```

### 5.2 PxL 스크립트 — 커스텀 분석

```python
# 느린 HTTP 요청 상위 10개 조회 (PxL 스크립트)
import px

# 최근 5분간 HTTP 요청 수집
df = px.DataFrame(table='http_events', start_time='-5m')

# 지연 시간으로 정렬
df = df[df['latency'] > 1000]  # 1초 이상
df = df.groupby(['service', 'req_path']).agg(
    avg_latency=('latency', px.mean),
    count=('latency', px.count),
    p99_latency=('latency', px.quantiles(0.99))
)
df = df.sort(by='p99_latency', ascending=False)
df = df.head(10)
px.display(df, 'slow_requests')
```

---

## 6. Tetragon — 보안 이벤트 추적

Cilium의 Tetragon은 eBPF로 보안 관련 커널 이벤트를 추적합니다.

```bash
# Tetragon 설치
helm install tetragon cilium/tetragon -n kube-system
kubectl rollout status -n kube-system ds/tetragon

# 프로세스 실행 이벤트 모니터링
kubectl exec -n kube-system ds/tetragon -c tetragon -- \
  tetra getevents --output compact | grep process_exec

# 특정 파드의 파일 접근 추적
kubectl exec -n kube-system ds/tetragon -c tetragon -- \
  tetra getevents --output compact \
  --namespace production \
  --pod my-app
```

```yaml
# TracingPolicy — 커스텀 추적 정책
apiVersion: cilium.io/v1alpha1
kind: TracingPolicy
metadata:
  name: track-sensitive-files
spec:
  kprobes:
  - call: "security_file_open"
    syscall: false
    args:
    - index: 0
      type: "file"
    selectors:
    - matchArgs:
      - index: 0
        operator: "Prefix"
        values:
        - "/etc/passwd"
        - "/etc/shadow"
        - "/root/.ssh"
      matchActions:
      - action: Sigkill    # 즉시 프로세스 종료!
      # 또는 action: Override (시스템 콜 차단)
```

---

## 7. eBPF 성능 프로파일링 — Parca

```bash
# Parca (지속적 프로파일링) 설치
kubectl apply -f https://github.com/parca-dev/parca/releases/latest/download/kubernetes-manifest.yaml
kubectl apply -f https://github.com/parca-dev/parca-agent/releases/latest/download/kubernetes-manifest.yaml

# Parca UI 포트포워딩
kubectl port-forward -n parca svc/parca 7070:7070

# 브라우저에서 http://localhost:7070
# → Flame Graph로 CPU 시간을 가장 많이 소모하는 함수 확인
# → 시간대별 비교 (최근 5분 vs 어제 이 시간)
```

---

## 8. 통합 관찰 가능성 스택

```
eBPF 기반 완전한 관찰 가능성 스택:

┌────────────────────────────────────────────────────────┐
│  수집 레이어                                           │
│  Cilium + Hubble   → 네트워크 플로우, L7 트래픽        │
│  Tetragon          → 보안/감사 이벤트                  │
│  Parca Agent       → CPU/메모리 프로파일링             │
└──────────────┬─────────────────────────────────────────┘
               │
┌──────────────▼─────────────────────────────────────────┐
│  저장 & 쿼리 레이어                                    │
│  Prometheus        → 메트릭 (hubble_*, tetragon_*)     │
│  Loki              → 로그 (Tetragon JSON 이벤트)       │
│  Parca             → 프로파일 데이터                   │
└──────────────┬─────────────────────────────────────────┘
               │
┌──────────────▼─────────────────────────────────────────┐
│  시각화 레이어                                         │
│  Grafana           → 대시보드 (메트릭 + 로그 연동)     │
│  Hubble UI         → 서비스 맵, 실시간 플로우          │
└────────────────────────────────────────────────────────┘
```

---

## 9. 트러블슈팅

* **Hubble이 L7 데이터를 수집 못 함:**
  ```bash
  # L7 가시성 활성화 확인
  kubectl get ciliumnetworkpolicy -A
  # → L7 정책이 있어야 Hubble이 L7 데이터 추적 가능

  # 또는 어노테이션으로 활성화
  kubectl annotate pod my-pod \
    "policy.cilium.io/proxy-visibility"="><port>/TCP/HTTP"
  ```

* **eBPF 프로그램 로드 실패:**
  ```bash
  # 커널 버전 확인 (eBPF는 4.9+, 완전한 기능은 5.8+)
  uname -r

  # eBPF 프로그램 상태 확인
  bpftool prog list | grep cilium

  # Cilium 에이전트 로그
  kubectl logs -n kube-system ds/cilium | grep "eBPF\|BPF" | tail -20
  ```

* **Pixie 설치 후 데이터 없음:**
  ```bash
  # Pixie 에이전트 상태 확인
  px get viziers

  # 커널 헤더 필요한 경우 (일부 배포판)
  kubectl get pods -n pl | grep kelvin   # Pixie 에이전트
  kubectl logs -n pl <kelvin-pod> | grep error
  ```
