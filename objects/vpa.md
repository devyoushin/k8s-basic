# VPA — Vertical Pod Autoscaler

## 1. 개요 및 비유

Pod의 CPU/메모리 requests/limits를 실제 사용량에 맞게 자동으로 조정하는 리소스 최적화 컨트롤러.

💡 비유: HPA가 직원 수를 늘리는 것이라면, VPA는 기존 직원의 책상 크기(리소스 할당)를 업무량에 맞게 자동으로 조정하는 것.

---

## 2. 핵심 설명

### 2.1 동작 원리

#### VPA 구성 요소

```
[VPA Recommender]  ← Metrics Server에서 과거 사용량 수집
      │ 권장값 계산
      ▼
[VPA Admission Controller]  ← Pod 생성/재시작 시 requests 자동 주입
      │
      ▼
[VPA Updater]  ← 현재 실행 중인 Pod 축출(Evict) → 새 requests로 재시작
```

#### VPA 업데이트 모드

| 모드 | 동작 | 사용 시점 |
|------|------|----------|
| `Off` | 권장값 계산만, 적용 안 함 | 초기 분석, 모니터링 |
| `Initial` | Pod 최초 생성 시에만 적용 | 안정성 우선 환경 |
| `Auto` | 권장값 벗어나면 Pod 재시작 | 일반 권장 |
| `Recreate` | Auto와 동일 (명시적) | Auto와 동일 |

#### HPA와 VPA 충돌 주의

- **동일 Deployment에 HPA + VPA(Auto) 동시 적용 금지**
- HPA가 CPU 기준 스케일 아웃하는 동안 VPA가 CPU requests를 변경 → 서로 충돌
- **해결책**: VPA는 `updateMode: Off`로 권장값만 참고하거나, 메모리만 VPA 관리

#### VPA 권장값 계산 알고리즘

- 최근 8일간 사용량의 히스토그램 기반
- 기본값: 90th 퍼센타일 사용량 + 버퍼
- 최솟값/최댓값 제한 가능 (`minAllowed`, `maxAllowed`)

---

### 2.2 YAML 적용 예시

#### VPA 설치 (Helm)

```bash
helm repo add fairwinds-stable https://charts.fairwinds.com/stable
helm install vpa fairwinds-stable/vpa \
  --namespace vpa \
  --create-namespace \
  --set "recommender.enabled=true" \
  --set "updater.enabled=true" \
  --set "admissionController.enabled=true"
```

#### VPA — Off 모드 (권장값 모니터링만)

```yaml
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: payment-svc-vpa
  namespace: payments
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: payment-svc
  updatePolicy:
    updateMode: "Off"   # 권장값만 계산, 적용 안 함
  resourcePolicy:
    containerPolicies:
      - containerName: payment-svc
        minAllowed:
          cpu: "100m"
          memory: "128Mi"
        maxAllowed:
          cpu: "4"
          memory: "8Gi"
        controlledResources: ["cpu", "memory"]
```

#### VPA — Auto 모드 (자동 조정)

```yaml
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: batch-worker-vpa
  namespace: batch
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: batch-worker
  updatePolicy:
    updateMode: "Auto"
    minReplicas: 2   # 최소 레플리카 수 이하일 때 Evict 안 함
  resourcePolicy:
    containerPolicies:
      - containerName: batch-worker
        minAllowed:
          cpu: "250m"
          memory: "256Mi"
        maxAllowed:
          cpu: "8"
          memory: "16Gi"
        # 메모리만 VPA로 관리 (CPU는 HPA에 위임)
        controlledResources: ["memory"]
        controlledValues: RequestsAndLimits
```

#### HPA + VPA 공존 패턴 (메모리만 VPA)

```yaml
# HPA: CPU 기반 수평 확장
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: api-svc-hpa
  namespace: api
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: api-svc
  minReplicas: 2
  maxReplicas: 20
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
---
# VPA: 메모리만 수직 조정
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: api-svc-vpa
  namespace: api
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: api-svc
  updatePolicy:
    updateMode: "Auto"
  resourcePolicy:
    containerPolicies:
      - containerName: api-svc
        controlledResources: ["memory"]  # CPU는 HPA가 관리
        minAllowed:
          memory: "256Mi"
        maxAllowed:
          memory: "4Gi"
```

---

### 2.3 Best Practice

- **초기 도입은 `Off` 모드**: 1~2주 데이터 수집 후 권장값 검토, 안정성 확인 후 `Auto` 전환
- **PodDisruptionBudget 필수 설정**: VPA Updater가 Pod를 축출할 때 가용성 보호
- **StatefulSet에는 VPA 신중히 적용**: 재시작 시 데이터 정합성 위험 → `Initial` 모드 권장
- **JVM 애플리케이션 주의**: Java Heap은 `-Xmx`로 고정되어 있어 메모리 증가가 즉시 반영 안 됨 → Heap 설정도 함께 조정 필요

---

## 3. 트러블슈팅

### 3.1 주요 이슈

#### VPA 권장값이 너무 낮게 책정됨

**증상**: VPA 적용 후 Pod OOMKilled 발생

**원인**: 권장값 계산 기간(8일) 내에 피크 트래픽이 없었음

**해결 방법**:
```bash
# 현재 VPA 권장값 확인
kubectl describe vpa <VPA_NAME> -n <NAMESPACE>
# Status.Recommendation.ContainerRecommendations 항목 확인

# maxAllowed 상향 조정 후 minAllowed로 하한선 보장
kubectl patch vpa <VPA_NAME> -n <NAMESPACE> --type='json' \
  -p='[{"op":"replace","path":"/spec/resourcePolicy/containerPolicies/0/minAllowed/memory","value":"512Mi"}]'
```

#### VPA Updater가 Pod를 계속 재시작시킴

**증상**: Pod가 수시로 Evict되어 서비스 불안정

**원인**: 실제 사용량이 지속적으로 권장값을 벗어남 (예: 메모리 누수)

**해결 방법**:
```bash
# VPA Updater 로그에서 Evict 원인 확인
kubectl logs -n vpa deploy/vpa-updater | grep -i "evict\|pod"

# 임시 조치: updateMode를 Off로 변경
kubectl patch vpa <VPA_NAME> -n <NAMESPACE> \
  --type='merge' \
  -p='{"spec":{"updatePolicy":{"updateMode":"Off"}}}'

# 근본 원인: 메모리 누수 여부 점검
kubectl top pod -n <NAMESPACE> --containers
```

---

### 3.2 자주 발생하는 문제

#### VPA Admission Controller 미설치로 Initial/Auto 모드 무효

**증상**: VPA를 Auto로 설정해도 Pod requests가 변경되지 않음

**원인**: VPA Admission Controller(Webhook)가 설치되지 않음

**해결 방법**:
```bash
kubectl get pods -n vpa | grep admission
# admission controller Pod 없으면 재설치
helm upgrade vpa fairwinds-stable/vpa -n vpa \
  --set "admissionController.enabled=true"
```

---

## 4. 모니터링 및 확인

```bash
# VPA 권장값 조회
kubectl describe vpa <VPA_NAME> -n <NAMESPACE>

# 현재 Pod 리소스 사용량 확인
kubectl top pod -n <NAMESPACE> --containers

# VPA가 적용한 실제 requests 확인
kubectl get pod <POD_NAME> -n <NAMESPACE> \
  -o jsonpath='{.spec.containers[*].resources}'

# VPA Recommender 로그 (권장값 계산 과정)
kubectl logs -n vpa deploy/vpa-recommender --tail=50

# 전체 VPA 목록
kubectl get vpa -A
```

---

## 5. TIP

- **Goldilocks 도구 활용**: Fairwinds Goldilocks를 설치하면 클러스터 전체 VPA 권장값을 웹 대시보드로 시각화 가능 (`Off` 모드 VPA를 자동 생성해 줌)
- **requests만 조정 vs requests+limits**: `controlledValues: RequestsOnly`로 limits는 그대로 두고 requests만 조정 가능 → limits 비율 유지에 유리
- **KEDA + VPA**: KEDA로 이벤트 기반 스케일 아웃 + VPA로 메모리 최적화를 조합하면 비용 효율적인 오토스케일링 구성 가능
