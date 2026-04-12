## 1. 개요 및 비유

**배포 전략(Deployment Strategy)**은 새 버전의 파드를 어떤 순서와 속도로 교체할지 결정하는 정책입니다.

💡 **비유하자면 식당 리모델링 방식과 같습니다.**
- **Rolling Update**: 구역을 나눠 한 쪽은 영업하면서 순서대로 리모델링
- **Recreate**: 전체 휴업 후 한꺼번에 리모델링
- **Blue/Green**: 옆 건물에 새 식당을 완성 후 간판(트래픽)만 교체
- **Canary**: 새 메뉴를 일부 테이블에만 먼저 제공해서 반응 확인 후 전체 적용

---

## 2. 핵심 설명

### Rolling Update (기본 전략)
새 파드를 점진적으로 올리면서 기존 파드를 내리는 방식입니다.

| 파라미터 | 의미 | 기본값 |
|---|---|---|
| `maxUnavailable` | 동시에 Unavailable 상태가 될 수 있는 파드 수 (절댓값 또는 %) | 25% |
| `maxSurge` | 원하는 복제본 수를 초과해서 추가 생성할 수 있는 파드 수 | 25% |

**예시: replicas=4, maxUnavailable=25%, maxSurge=25%일 때**
- 동시에 1개 파드까지 Unavailable 허용 → 기존 파드 1개 종료
- 동시에 1개 파드까지 초과 생성 허용 → 새 파드 1개 추가 생성
- 최소 3개(75%)의 파드가 항상 Running 상태를 유지해야 다음 단계 진행

### Recreate
모든 기존 파드를 종료한 뒤, 새 파드를 전부 생성합니다.
- 장점: 구버전/신버전 동시 운영 없음, 설정 단순
- 단점: 파드 전환 중 다운타임 발생

---

## 3. YAML 적용 예시

### Rolling Update (권장 - 무중단 배포)
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app
spec:
  replicas: 4
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0   # 업데이트 중 서비스 불가 파드 0개 (무중단 보장)
      maxSurge: 1         # 최대 5개(4+1)까지 파드 허용
  template:
    spec:
      containers:
      - name: app
        image: my-app:2.0
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        readinessProbe:           # 반드시 설정: Ready 상태 정확히 판단
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
          failureThreshold: 3
      terminationGracePeriodSeconds: 60   # 기존 파드 graceful shutdown 시간
```

### Recreate (다운타임 허용 시)
```yaml
spec:
  strategy:
    type: Recreate   # 기존 파드 전부 종료 후 새 파드 생성
```

### Blue/Green (Argo Rollouts 또는 수동 서비스 전환)
```yaml
# Blue Deployment (현재 운영 중)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app-blue
spec:
  replicas: 4
  template:
    metadata:
      labels:
        app: web-app
        version: blue      # 라벨로 버전 구분
    spec:
      containers:
      - name: app
        image: my-app:1.0
---
# Green Deployment (새 버전 준비)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app-green
spec:
  replicas: 4
  template:
    metadata:
      labels:
        app: web-app
        version: green
    spec:
      containers:
      - name: app
        image: my-app:2.0
---
# Service의 selector를 blue → green으로 변경하면 트래픽 전환
apiVersion: v1
kind: Service
metadata:
  name: web-app
spec:
  selector:
    app: web-app
    version: green   # blue에서 green으로 변경하면 즉시 트래픽 전환
```

---

## 4. 트러블 슈팅

### 사례: EKS에서 Resource Request 증가 배포 시 NLB Unhealthy 발생

**상황:**
- `memory request`: 250Mi → 1000Mi 로 증가 후 롤링 업데이트
- `maxUnavailable: 25%` (기본값)
- 배포 직후 NLB(Network Load Balancer) 헬스체크 unhealthy 감지

**원인 분석:**

Rolling Update 과정에서 발생하는 실제 흐름:

```
[1단계] 기존 파드(250Mi) 1개 → Terminating
         ↓ K8s가 Endpoint에서 즉시 제거
         ↓ NLB가 해당 타깃을 드레이닝(connection draining)

[2단계] 새 파드(1000Mi) 생성 시도
         ↓ 노드의 할당 가능한 메모리 부족
         ↓ 파드가 Pending 상태로 대기 (스케줄 불가)

[3단계] Running 파드 수 감소 (4개 → 3개 이하)
         ↓ NLB 타깃 그룹의 healthy 타깃 수 감소
         ↓ 헬스체크 임계값 미달 → NLB unhealthy 판정
```

**"기존 파드가 살아있으니 괜찮지 않나?"는 오해:**
- Rolling Update에서 기존 파드는 **먼저 Terminating** 됩니다 (maxUnavailable 기준)
- 파드가 Terminating 상태가 되는 순간 K8s는 해당 파드를 Endpoint에서 제거
- NLB는 이미 해당 타깃을 unhealthy로 처리하고 트래픽을 끊습니다
- 새 파드가 Ready가 되어야 새 타깃이 등록되는데, Pending이면 그 공백이 발생합니다

**해결 방법:**

```yaml
# 방법 1: maxUnavailable: 0 설정 (권장)
# 새 파드가 Ready가 되어야만 기존 파드를 종료 → 공백 없음
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxUnavailable: 0   # 기존 파드 먼저 종료하지 않음
    maxSurge: 1         # 새 파드를 먼저 생성

# 방법 2: PodDisruptionBudget으로 최소 가용 파드 수 강제
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: web-app-pdb
spec:
  minAvailable: 3      # 항상 최소 3개 파드 유지
  selector:
    matchLabels:
      app: web-app
```

```yaml
# 방법 3: 노드 리소스 여유 확보 후 배포
# Cluster Autoscaler 또는 Karpenter가 노드를 미리 확장하도록
# pod의 priorityClass나 PodSchedulingReadinessGate 활용

# 배포 전 노드 여유 확인
# kubectl describe nodes | grep -A5 "Allocated resources"

# 방법 4: 단계적 resource request 증가
# 한 번에 250Mi → 1000Mi 올리지 말고
# 250Mi → 500Mi → 1000Mi 처럼 단계적으로 올려서 CA가 노드 확장할 시간 부여
```

**NLB 헬스체크와 readinessProbe 연동 확인:**
```bash
# 파드 Ready 상태 확인
kubectl get pods -o wide --watch

# NLB 타깃 그룹 헬스체크 상태 확인 (AWS CLI)
aws elbv2 describe-target-health \
  --target-group-arn <target-group-arn>

# Endpoint 슬라이스 확인 (Ready 파드가 등록되었는지)
kubectl get endpointslices -l kubernetes.io/service-name=<service-name> -o yaml

# 이벤트 확인 (스케줄 실패 원인)
kubectl get events --sort-by='.lastTimestamp' | grep -i "failed\|insufficient\|pending"
```

---

### 배포 전략 선택 가이드

| 전략 | 다운타임 | 리소스 | 트래픽 제어 | 롤백 속도 | 권장 상황 |
|---|---|---|---|---|---|
| Rolling Update (maxUnavailable>0) | 없음 | 보통 | 제한적 | 느림 | 일반 무중단 배포 |
| Rolling Update (maxUnavailable=0) | 없음 | 더 많이 필요 | 제한적 | 느림 | 가용성 중요한 서비스 |
| Recreate | 있음 | 적음 | 없음 | 빠름 | 개발환경, DB 마이그레이션 |
| Blue/Green | 없음 | 2배 필요 | 완전 제어 | 즉시 | 중요 서비스, 검증 필요 |
| Canary | 없음 | 조금 더 필요 | 세밀한 제어 | 즉시 | 새 기능 점진적 검증 |

**Resource Request 크게 올릴 때 체크리스트:**
- [ ] 노드 여유 메모리/CPU 확인 (`kubectl describe nodes`)
- [ ] `maxUnavailable: 0` 설정으로 안전 배포
- [ ] Cluster Autoscaler / Karpenter 정상 동작 확인
- [ ] PodDisruptionBudget 설정 여부 확인
- [ ] readinessProbe 설정으로 NLB 타깃 등록 시점 정확히 제어
- [ ] 배포 중 NLB 타깃 그룹 헬스체크 모니터링
