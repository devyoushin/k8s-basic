## 1. 개요 및 비유
**관찰 가능성(Observability)**은 시스템의 내부 상태를 외부 출력(로그, 메트릭, 트레이싱)으로 파악하는 능력입니다. 쿠버네티스 환경에서는 파드가 언제든 죽고 교체되기 때문에, 3가지 신호를 제대로 수집하지 않으면 장애 원인을 찾기가 매우 어렵습니다.

💡 **비유하자면 '병원의 환자 모니터링 시스템'과 같습니다.**
로그는 환자의 진료 기록(무슨 일이 있었는지), 메트릭은 심박수/혈압 모니터(지금 상태), 트레이싱은 의뢰서 경로 추적(어느 과에서 어느 과로 갔는지)입니다. 셋 중 하나만 있어도 진단이 어렵습니다.

## 2. 핵심 설명

### 관찰 가능성의 3대 신호

| 신호 | 설명 | 주요 도구 |
|---|---|---|
| **Logs (로그)** | 이벤트의 타임스탬프 기록 | Fluentd, Fluent Bit, Loki |
| **Metrics (메트릭)** | 시계열 수치 데이터 | Prometheus, Grafana |
| **Traces (트레이싱)** | 요청의 전체 흐름 추적 | Jaeger, Tempo, Zipkin |

### 쿠버네티스 로깅 아키텍처

```
[컨테이너 stdout/stderr]
        ↓
[노드의 /var/log/containers/]  ← kubelet이 관리
        ↓
[DaemonSet: Fluent Bit]        ← 각 노드에서 로그 수집
        ↓
[중앙 로그 저장소: Loki / OpenSearch / CloudWatch]
        ↓
[시각화: Grafana / Kibana]
```

**컨테이너 로그 모범 사례:**
- 반드시 **stdout/stderr**로 출력 (파일에 쓰지 않음)
- 구조화 로그(JSON 형식) 사용 → 쿼리/필터링 용이
- 로그 레벨 환경변수로 제어 (`LOG_LEVEL=INFO`)

### 메트릭 스택 (kube-prometheus-stack)

```
[cAdvisor]           → 컨테이너 리소스 메트릭 (CPU, Memory, Network)
[kubelet /metrics]   → 노드/파드 상태 메트릭
[kube-state-metrics] → K8s 오브젝트 상태 메트릭 (Deployment 상태 등)
[앱 /metrics 엔드포인트] → 앱 커스텀 메트릭
        ↓ (Prometheus가 Pull 방식으로 수집)
[Prometheus]         → 저장 + 알람(AlertManager)
        ↓
[Grafana]            → 시각화 대시보드
```

### 분산 트레이싱 동작 원리

```
사용자 요청
    │ trace-id: abc123
    ▼
[frontend]  span: 15ms
    │ trace-id: abc123, parent-span: -
    ▼
[backend]   span: 8ms
    │ trace-id: abc123, parent-span: frontend
    ▼
[database]  span: 3ms
    trace-id: abc123, parent-span: backend

→ Jaeger에서 trace-id로 전체 흐름 조회 가능
```

## 3. YAML 적용 예시

### Fluent Bit DaemonSet (로그 수집 → CloudWatch)
```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: fluent-bit
  namespace: kube-system
spec:
  selector:
    matchLabels:
      name: fluent-bit
  template:
    metadata:
      labels:
        name: fluent-bit
    spec:
      serviceAccountName: fluent-bit
      tolerations:
      - key: node-role.kubernetes.io/control-plane
        effect: NoSchedule
      containers:
      - name: fluent-bit
        image: fluent/fluent-bit:2.2
        resources:
          requests:
            cpu: "50m"
            memory: "64Mi"
          limits:
            cpu: "200m"
            memory: "128Mi"
        volumeMounts:
        - name: varlog
          mountPath: /var/log
        - name: config
          mountPath: /fluent-bit/etc/
      volumes:
      - name: varlog
        hostPath:
          path: /var/log
      - name: config
        configMap:
          name: fluent-bit-config
```

### Fluent Bit ConfigMap (구조화 로그 파싱)
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: fluent-bit-config
  namespace: kube-system
data:
  fluent-bit.conf: |
    [SERVICE]
        Flush         5
        Log_Level     info
        Parsers_File  parsers.conf

    [INPUT]
        Name              tail
        Path              /var/log/containers/*.log
        Parser            docker
        Tag               kube.*
        Refresh_Interval  5
        Mem_Buf_Limit     50MB
        Skip_Long_Lines   On

    [FILTER]
        Name                kubernetes
        Match               kube.*
        Kube_URL            https://kubernetes.default.svc:443
        Merge_Log           On      # JSON 로그를 파싱하여 필드 분리
        Keep_Log            Off
        K8S-Logging.Parser  On

    [OUTPUT]
        Name              cloudwatch_logs
        Match             kube.*
        region            ap-northeast-2
        log_group_name    /eks/my-cluster
        log_stream_prefix from-fluent-bit-
        auto_create_group true

  parsers.conf: |
    [PARSER]
        Name        docker
        Format      json
        Time_Key    time
        Time_Format %Y-%m-%dT%H:%M:%S.%L
```

### Prometheus ServiceMonitor (앱 메트릭 수집)
```yaml
# 앱이 /metrics 엔드포인트를 제공하면 Prometheus가 자동 수집
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: my-app-monitor
  namespace: production
  labels:
    release: prometheus   # kube-prometheus-stack이 이 라벨로 ServiceMonitor 탐색
spec:
  selector:
    matchLabels:
      app: my-app
  endpoints:
  - port: metrics         # Service의 포트 이름
    path: /metrics
    interval: 30s
    scrapeTimeout: 10s
```

### PrometheusRule (알람 규칙)
```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: my-app-alerts
  namespace: production
  labels:
    release: prometheus
spec:
  groups:
  - name: my-app
    interval: 1m
    rules:
    # 에러율 알람
    - alert: HighErrorRate
      expr: |
        sum(rate(http_requests_total{status=~"5.."}[5m]))
        /
        sum(rate(http_requests_total[5m])) > 0.05
      for: 5m           # 5분 이상 지속될 때만 알람
      labels:
        severity: critical
        team: backend
      annotations:
        summary: "에러율이 5%를 초과했습니다"
        description: "{{ $labels.namespace }}/{{ $labels.pod }} 에러율: {{ $value | humanizePercentage }}"

    # 파드 재시작 알람
    - alert: PodCrashLooping
      expr: increase(kube_pod_container_status_restarts_total[1h]) > 5
      for: 0m
      labels:
        severity: warning
      annotations:
        summary: "파드가 1시간 내 5회 이상 재시작됨"
```

### OpenTelemetry 트레이싱 설정 (앱 사이드카)
```yaml
spec:
  containers:
  - name: app
    image: my-app:1.0
    env:
    # OpenTelemetry SDK가 이 환경변수를 자동으로 읽음
    - name: OTEL_SERVICE_NAME
      value: "my-app"
    - name: OTEL_EXPORTER_OTLP_ENDPOINT
      value: "http://otel-collector:4317"  # OpenTelemetry Collector 주소
    - name: OTEL_TRACES_SAMPLER
      value: "parentbased_traceidratio"
    - name: OTEL_TRACES_SAMPLER_ARG
      value: "0.1"   # 10% 샘플링 (프로덕션 트래픽 많을 때)
```

## 4. 트러블 슈팅

* **파드 로그가 수집되지 않음 (Fluent Bit):**
  ```bash
  # Fluent Bit 파드 로그 확인
  kubectl logs -n kube-system -l name=fluent-bit --tail=50

  # 파드 로그가 노드에 있는지 직접 확인
  kubectl exec -n kube-system <fluent-bit-pod> -- ls /var/log/containers/
  ```
  * Fluent Bit의 `Mem_Buf_Limit` 초과로 로그가 드랍될 수 있습니다. 값을 늘리거나 `Storage.type filesystem`으로 디스크 버퍼를 사용하세요.

* **Prometheus가 타겟을 찾지 못함 (`0/1 up`):**
  ```bash
  # Prometheus UI → Status → Targets 에서 에러 메시지 확인
  kubectl port-forward svc/prometheus 9090:9090 -n monitoring

  # ServiceMonitor 라벨이 Prometheus의 serviceMonitorSelector와 일치하는지 확인
  kubectl get prometheus -n monitoring -o yaml | grep serviceMonitorSelector
  ```

* **메트릭은 있는데 알람이 오지 않음:**
  * `for: 5m` 조건 때문에 5분 미만 지속된 이슈는 알람이 발생하지 않습니다.
  * AlertManager의 `routes` 설정에서 해당 `severity` 라벨을 처리하는 수신자(receiver)가 있는지 확인하세요.
  * Prometheus UI → Alerts 탭에서 알람 상태(`inactive`/`pending`/`firing`)를 확인하세요.

* **트레이싱 데이터가 너무 많아 저장 비용 급등:**
  * 샘플링 비율을 낮추세요. `OTEL_TRACES_SAMPLER_ARG: "0.01"` (1% 샘플링)
  * 에러 요청은 100% 샘플링하고 정상 요청만 줄이는 **Tail Sampling** 전략을 OpenTelemetry Collector에 설정하세요.
