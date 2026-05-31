## 1. 개요 및 비유

오토스케일링은 트래픽 변화에 따라 파드 수를 자동으로 조정합니다. HPA는 메트릭 기반, KEDA는 이벤트 기반으로 동작합니다.

💡 **비유하자면 셀프 계산대 수 자동 조절과 같습니다.**
손님(요청)이 많아지면 계산대(파드)를 늘리고, 한산해지면 줄입니다. KEDA는 대기열(Kafka, SQS)에 주문이 쌓이기 시작할 때 미리 계산대를 열 수 있습니다.

---

## 2. HPA 내부 알고리즘

### 2.1 스케일링 계산 공식

```
목표 파드 수 = ceil[ 현재 파드 수 × (현재 메트릭 값 / 목표 메트릭 값) ]

예: CPU 목표 50%, 현재 3개 파드, 현재 평균 CPU 90%
  → ceil[ 3 × (90 / 50) ] = ceil[5.4] = 6개

예: CPU 목표 50%, 현재 6개 파드, 현재 평균 CPU 20%
  → ceil[ 6 × (20 / 50) ] = ceil[2.4] = 3개

허용 오차 (기본 10%):
  현재 비율이 90%~110% 사이면 스케일링 안 함
  → 작은 변동에 과민 반응 방지
```

### 2.2 HPA 제어 루프 흐름

```
HPA Controller (15초 주기):
        │
        ▼
Metrics Server에서 현재 메트릭 수집
  - CPU/Memory: metrics.k8s.io API (Metrics Server)
  - Custom Metrics: custom.metrics.k8s.io (Prometheus Adapter 등)
  - External Metrics: external.metrics.k8s.io (KEDA, Datadog 등)
        │
        ▼
목표 파드 수 계산
        │
        ▼
스케일업/다운 쿨다운 확인
  - scaleUp: 0초 (즉시, 기본값)
  - scaleDown: 300초 (5분 대기, 기본값)
        │
        ▼
[스케일링 필요하면]
Deployment/StatefulSet의 replicas 업데이트
```

```bash
# HPA 현재 상태 확인
kubectl get hpa -w
# NAME         REFERENCE           TARGETS   MINPODS   MAXPODS   REPLICAS
# my-hpa       Deployment/my-app   45%/50%   2         10        3

# HPA 상세 정보 (이벤트, 조건 포함)
kubectl describe hpa my-hpa
# Conditions:
#   AbleToScale: True
#   ScalingActive: True
#   ScalingLimited: False (min/max에 의해 제한 중이면 True)
```

### 2.3 HPA 설정 심화

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: my-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-app
  minReplicas: 2
  maxReplicas: 20

  metrics:
  # CPU 기반
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 50   # requests 대비 50%

  # 메모리 기반
  - type: Resource
    resource:
      name: memory
      target:
        type: AverageValue
        averageValue: 400Mi      # 파드당 평균 400Mi

  # 커스텀 메트릭 (Prometheus 등)
  - type: Pods
    pods:
      metric:
        name: http_requests_per_second   # Prometheus 메트릭 이름
      target:
        type: AverageValue
        averageValue: "100"    # 파드당 초당 100 요청

  behavior:
    scaleUp:
      stabilizationWindowSeconds: 0    # 즉시 스케일업
      policies:
      - type: Percent
        value: 100              # 한 번에 최대 현재의 2배까지
        periodSeconds: 60
      - type: Pods
        value: 4                # 또는 한 번에 최대 4개
        periodSeconds: 60
      selectPolicy: Max         # 위 두 정책 중 큰 값 선택

    scaleDown:
      stabilizationWindowSeconds: 300  # 5분 안정화 후 스케일다운
      policies:
      - type: Percent
        value: 10               # 한 번에 최대 10%씩만 감소
        periodSeconds: 60
```

---

## 3. VPA (Vertical Pod Autoscaler)

### 3.1 VPA 동작 모드

```
VPA 구성 요소:
- VPA Recommender: 히스토리 기반으로 적정 requests 계산
- VPA Admission Plugin: 파드 생성 시 requests 자동 조정
- VPA Updater: 기존 파드 강제 재시작으로 requests 업데이트

동작 모드:
Off:      추천만 계산, 실제 적용 안 함 (관찰용)
Initial:  새 파드 생성 시에만 적용
Auto:     실행 중인 파드도 재시작하여 적용 (위험할 수 있음)
```

```yaml
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: my-vpa
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-app
  updatePolicy:
    updateMode: "Off"    # 먼저 Off로 추천값만 확인
  resourcePolicy:
    containerPolicies:
    - containerName: app
      minAllowed:
        cpu: 100m
        memory: 128Mi
      maxAllowed:
        cpu: 4
        memory: 4Gi
```

```bash
# VPA 추천 확인
kubectl describe vpa my-vpa
# Recommendation:
#   Container Recommendations:
#     Container Name: app
#     Lower Bound:    cpu 100m, memory 200Mi
#     Target:         cpu 300m, memory 512Mi   ← 이 값으로 설정 권장
#     Upper Bound:    cpu 1000m, memory 2Gi
```

**주의:** HPA(수평)와 VPA(수직)를 CPU/Memory 기준으로 동시에 사용하면 충돌합니다. CPU는 HPA, Memory는 VPA로 분리하거나, VPA는 Off 모드로만 사용하는 것이 안전합니다.

---

## 4. KEDA (Kubernetes Event-Driven Autoscaling)

### 4.1 KEDA 아키텍처

```
KEDA 구성 요소:
- ScaledObject: 스케일링 대상과 트리거 정의
- KEDA Operator: ScaledObject를 HPA로 변환
- KEDA Metrics Server: 외부 메트릭을 HPA API로 노출
- Scaler: 각 이벤트 소스 연동 (Kafka, SQS, Redis 등)

KEDA → HPA 변환:
ScaledObject (Kafka topic, lag 100)
    │
    ▼
KEDA Operator가 HPA 생성
HPA: externalMetrics → kafka_consumer_lag
    │
    ▼
파드 수 자동 조정
```

### 4.2 KEDA 스케일러 예시

```yaml
# Kafka Consumer Lag 기반 스케일링
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: kafka-consumer-scaler
spec:
  scaleTargetRef:
    name: my-consumer-deployment
  minReplicaCount: 0    # 0으로 스케일다운 가능! (HPA는 최소 1)
  maxReplicaCount: 30
  cooldownPeriod: 300   # 스케일다운 대기 시간 (초)
  triggers:
  - type: kafka
    metadata:
      bootstrapServers: kafka.default.svc.cluster.local:9092
      consumerGroup: my-consumer-group
      topic: my-topic
      lagThreshold: "100"  # 파드당 lag 100이 목표
      offsetResetPolicy: latest

---
# SQS 기반 스케일링
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: sqs-scaler
spec:
  scaleTargetRef:
    name: sqs-worker
  minReplicaCount: 0
  maxReplicaCount: 10
  triggers:
  - type: aws-sqs-queue
    metadata:
      queueURL: https://sqs.ap-northeast-2.amazonaws.com/123/my-queue
      queueLength: "5"      # 파드당 메시지 5개
      awsRegion: ap-northeast-2
    authenticationRef:
      name: keda-aws-credentials

---
# Prometheus 메트릭 기반 스케일링
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: prometheus-scaler
spec:
  scaleTargetRef:
    name: my-api
  triggers:
  - type: prometheus
    metadata:
      serverAddress: http://prometheus.monitoring.svc:9090
      metricName: http_requests_per_second
      query: sum(rate(http_requests_total[2m]))
      threshold: "100"   # 초당 100 요청당 파드 1개
```

### 4.3 Scale to Zero — 유휴 시 완전 종료

```
KEDA의 핵심 기능: minReplicaCount: 0 설정 시
  - 이벤트 없으면 → 0개로 스케일다운 (비용 절감)
  - 이벤트 발생하면 → 즉시 스케일업

주의사항:
  - 0→1 시작 시간 = Cold Start
  - HTTP 요청이면 첫 요청이 타임아웃될 수 있음
  - HTTP 애드온(http-add-on)으로 해결 가능:
    요청을 큐에 담아두고 파드 준비되면 전달
```

---

## 5. Cluster Autoscaler — 노드 자동 증감

```
파드 스케줄 실패 (Pending) 감지
        │
        ▼
Cluster Autoscaler (10초 주기 체크):
  - 어떤 노드 그룹을 늘리면 Pending 파드 해소 가능한지 계산
  - 가장 효율적인 노드 그룹 선택
        │
        ▼
클라우드 API로 노드 추가 (ASG, MIG, VMSS)
  - EC2 Launch Template 기반 노드 시작
  - 노드 Ready 상태 될 때까지 대기 (약 2~5분)
        │
        ▼
파드 스케줄링 재시도

스케일다운 조건:
  - 노드의 파드가 다른 노드에 모두 이동 가능할 때
  - 10분 이상 활용률 50% 이하 유지
  - PDB 위반하지 않을 때
```

```bash
# Cluster Autoscaler 로그
kubectl logs -n kube-system deployment/cluster-autoscaler | grep -E "scale|node"

# 스케일업 이벤트 확인
kubectl get events -A | grep TriggeredScaleUp

# 노드 스케일다운 방지 어노테이션
kubectl annotate node worker-3 cluster-autoscaler.kubernetes.io/scale-down-disabled=true
```

---

## 6. 트러블슈팅

* **HPA가 UNKNOWN 상태 (메트릭 수집 실패):**
  ```bash
  # Metrics Server 설치 확인
  kubectl get deployment metrics-server -n kube-system

  # Metrics Server 정상 동작 확인
  kubectl top nodes
  kubectl top pods

  # HPA 이벤트 확인
  kubectl describe hpa my-hpa | grep -A10 "Events:"
  # "FailedGetResourceMetric" → Metrics Server 문제
  ```

* **스케일다운이 안 됨:**
  ```bash
  # HPA stabilizationWindowSeconds 확인 (기본 5분 대기)
  kubectl get hpa my-hpa -o yaml | grep stabilization

  # PDB가 스케일다운을 막는지 확인
  kubectl get pdb
  kubectl describe pdb my-pdb

  # 특정 파드에 disruption 허용 여부
  kubectl get pod my-pod -o jsonpath='{.metadata.annotations}'
  # cluster-autoscaler.kubernetes.io/safe-to-evict: false 이면 CA가 못 줄임
  ```

* **KEDA ScaledObject가 작동 안 함:**
  ```bash
  # KEDA 오퍼레이터 로그
  kubectl logs -n keda deployment/keda-operator

  # ScaledObject 상태 확인
  kubectl describe scaledobject kafka-consumer-scaler
  # Conditions에서 Ready: False 이유 확인

  # KEDA가 생성한 HPA 확인
  kubectl get hpa | grep keda
  ```
