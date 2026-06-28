## 1. 개요 및 비유

Scheduled Scaling(스케줄 기반 스케일링)은 트래픽이 몰리는 시간이 예측될 때, 미리 Deployment(디플로이먼트)나 StatefulSet(스테이트풀셋)의 Pod 수를 늘리고 피크가 끝난 뒤 줄이는 운영 패턴이다.

💡 **비유하자면 '점심시간 전에 미리 계산대를 여는 것'과 같음.**
HPA(Horizontal Pod Autoscaler)는 줄이 길어진 뒤 계산대를 더 여는 방식이고, Scheduled Scaling은 매일 12시에 손님이 몰린다는 것을 알고 11시 50분부터 계산대를 미리 여는 방식이다.

---

## 2. 핵심 설명

### 2.1 언제 Scheduled Scaling을 쓰는가

| 상황 | 권장 방식 | 이유 |
|---|---|---|
| 매일 같은 시간 트래픽 피크 | KEDA cron scaler | 시간대별 최소 Pod 수 보장 |
| 주중/주말 트래픽 패턴 차이 | KEDA cron scaler 여러 개 구성 | cron 표현식으로 요일 분리 가능 |
| KEDA 미설치 클러스터 | CronJob + `kubectl scale` | 기본 Kubernetes 리소스만 사용 |
| HPA 반응이 늦음 | Scheduled Scaling + HPA 병행 | 피크 시작 전 warm capacity 확보 |
| 야간 비용 절감 | KEDA `minReplicaCount: 0` 또는 CronJob scale down | 사용량 없는 시간 Pod 축소 |
| 큐 기반 worker | KEDA SQS/Kafka scaler + cron scaler | 업무 시간 최소 처리량 보장 + 이벤트 기반 확장 |

Scheduled Scaling은 HPA를 대체하지 않는다. 예측 가능한 최소 용량을 미리 확보하고, 실제 부하가 더 크면 HPA/KEDA가 추가 확장하는 구조가 안정적이다.

### 2.2 선택 기준

| 방식 | 장점 | 단점 | 권장 상황 |
|---|---|---|---|
| KEDA cron scaler | HPA와 자연스럽게 통합, 시간대별 동적 최소 replica 역할 | KEDA 설치 필요 | 운영 표준으로 권장 |
| CronJob + `kubectl scale` | Kubernetes 기본 기능만 사용 | HPA와 replicas 충돌 가능, RBAC 필요 | KEDA 없는 환경의 단순 자동화 |
| GitOps scheduled sync | 변경 이력 관리 용이 | 즉시성·시간 정밀도는 도구 의존 | Argo CD/Flux 중심 운영 |
| 수동 `kubectl scale` | 가장 단순 | 사람 의존, 누락 위험 | 긴급 임시 대응 |

### 2.3 HPA와 함께 사용할 때 핵심 주의점

HPA가 걸린 Deployment의 `.spec.replicas`를 CronJob이 직접 수정하면, HPA controller가 다시 replicas를 계산해 덮어쓴다. 따라서 HPA가 있는 워크로드는 CronJob으로 직접 `kubectl scale`하기보다 KEDA cron scaler처럼 HPA metric으로 들어가는 방식을 우선 사용한다.

| 조합 | 결과 |
|---|---|
| HPA만 사용 | 부하 감지 후 반응. Pod 기동 시간만큼 지연 |
| CronJob `kubectl scale` + HPA | CronJob 값이 HPA에 의해 다시 변경될 수 있음 |
| KEDA cron scaler + CPU trigger | cron 시간대에는 최소 Pod 수 확보, CPU가 더 높으면 추가 확장 |
| KEDA cron scaler + `minReplicaCount: 0` | 시간대 밖에는 0까지 축소 가능 |

KEDA cron scaler의 `desiredReplicas`는 해당 시간대의 고정 replica라기보다 HPA 계산에서 동적 최소 replica처럼 동작한다. 다른 trigger가 더 많은 replica를 요구하면 더 큰 값이 선택된다.

### 2.4 KEDA cron scaler 동작 원리

KEDA cron scaler는 `start`와 `end` 사이의 시간대에 `desiredReplicas`를 요구하는 external metric을 HPA에 제공한다.

| 필드 | 의미 | 예시 |
|---|---|---|
| `timezone` | IANA Time Zone Database 기준 시간대 | `Asia/Seoul` |
| `start` | 스케일 아웃 시작 cron 표현식 | `50 8 * * 1-5` |
| `end` | 스케일 인 종료 cron 표현식 | `10 18 * * 1-5` |
| `desiredReplicas` | start~end 사이 확보할 replica 수 | `"10"` |
| `minReplicaCount` | 시간대 밖 최소 replica | `2` 또는 `0` |
| `maxReplicaCount` | 전체 최대 replica | `30` |
| `cooldownPeriod` | 스케일 다운 대기 시간 | `300` |

`start`와 `end`는 같으면 안 된다. cron 표현식은 `분 시 일 월 요일` 형식이다.

---

## 3. YAML 적용 예시

### 3.1 KEDA cron scaler — 업무 시간 최소 Pod 수 보장

매주 월~금 08:50~18:10(KST)에는 최소 10개 Pod를 유지하고, 그 외 시간에는 최소 2개로 줄이는 예시다.

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: my-app-scheduled-scaler
  namespace: default
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-app
  minReplicaCount: 2
  maxReplicaCount: 30
  pollingInterval: 30
  cooldownPeriod: 300
  triggers:
    - type: cron
      metadata:
        timezone: Asia/Seoul
        start: 50 8 * * 1-5
        end: 10 18 * * 1-5
        desiredReplicas: "10"
```

### 3.2 KEDA cron + CPU trigger 병행

예측 가능한 업무 시간에는 최소 10개를 유지하고, CPU 사용량이 높으면 최대 30개까지 늘리는 예시다.

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: my-app-cron-cpu-scaler
  namespace: default
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-app
  minReplicaCount: 2
  maxReplicaCount: 30
  pollingInterval: 30
  cooldownPeriod: 300
  triggers:
    - type: cron
      metadata:
        timezone: Asia/Seoul
        start: 50 8 * * 1-5
        end: 10 18 * * 1-5
        desiredReplicas: "10"
    - type: cpu
      metricType: Utilization
      metadata:
        value: "60"
```

이 구성에서 업무 시간 중 CPU trigger가 16개를 요구하면 16개로 증가한다. cron trigger는 최소 10개를 보장하는 역할을 한다.

### 3.3 야간 0개 스케일 다운

개발/스테이징처럼 야간 트래픽이 없는 워크로드는 시간대 밖에 0개까지 줄일 수 있다.

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: staging-app-office-hour-scaler
  namespace: staging
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: staging-app
  minReplicaCount: 0
  maxReplicaCount: 10
  cooldownPeriod: 300
  triggers:
    - type: cron
      metadata:
        timezone: Asia/Seoul
        start: 0 9 * * 1-5
        end: 0 19 * * 1-5
        desiredReplicas: "2"
```

0개에서 다시 올라올 때는 cold start가 발생한다. 운영 서비스에는 readinessProbe, startupProbe, 이미지 pull 시간, 초기 캐시 로딩 시간을 고려해 피크보다 충분히 이른 `start`를 잡는다.

### 3.4 CronJob + kubectl scale — KEDA 없는 환경

KEDA가 없는 클러스터에서는 CronJob이 `kubectl scale`을 실행하도록 구성할 수 있다. 이 방식은 HPA와 충돌할 수 있으므로 HPA가 없는 단순 워크로드나 임시 운영에만 사용한다.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: scheduled-scaler
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: deployment-scaler
  namespace: default
rules:
  - apiGroups:
      - apps
    resources:
      - deployments/scale
    verbs:
      - get
      - update
      - patch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: scheduled-scaler
  namespace: default
subjects:
  - kind: ServiceAccount
    name: scheduled-scaler
    namespace: default
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: deployment-scaler
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: my-app-scale-up
  namespace: default
spec:
  schedule: "50 8 * * 1-5"
  timeZone: "Asia/Seoul"
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 3
  jobTemplate:
    spec:
      backoffLimit: 1
      template:
        spec:
          serviceAccountName: scheduled-scaler
          restartPolicy: Never
          containers:
            - name: kubectl
              image: bitnami/kubectl:latest
              command:
                - kubectl
                - scale
                - deployment
                - my-app
                - --replicas=10
                - -n
                - default
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: my-app-scale-down
  namespace: default
spec:
  schedule: "10 18 * * 1-5"
  timeZone: "Asia/Seoul"
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 3
  jobTemplate:
    spec:
      backoffLimit: 1
      template:
        spec:
          serviceAccountName: scheduled-scaler
          restartPolicy: Never
          containers:
            - name: kubectl
              image: bitnami/kubectl:latest
              command:
                - kubectl
                - scale
                - deployment
                - my-app
                - --replicas=2
                - -n
                - default
```

`CronJob.spec.timeZone`은 Kubernetes 1.27부터 stable이다. 이전 버전은 kube-controller-manager의 로컬 시간대 기준으로 동작하므로 운영 표준 시간대를 별도로 확인한다.

### 3.5 requests 조정과 함께 적용하는 패턴

트래픽 피크 대응은 replica 수만 늘려서는 부족하다. `resources.requests`가 너무 낮으면 HPA CPU 사용률이 과대 계산되어 불필요하게 많이 늘고, 너무 높으면 Pod가 Pending 상태로 남아 노드 확장을 기다린다.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  namespace: default
spec:
  replicas: 2
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
        - name: app
          image: nginx:1.27
          resources:
            requests:
              cpu: 300m
              memory: 512Mi
            limits:
              cpu: "1"
              memory: 1Gi
          readinessProbe:
            httpGet:
              path: /
              port: 80
            initialDelaySeconds: 5
            periodSeconds: 5
          startupProbe:
            httpGet:
              path: /
              port: 80
            failureThreshold: 30
            periodSeconds: 2
```

피크 시간 전에 replica를 늘려도 readinessProbe가 없으면 준비되지 않은 Pod가 Service Endpoint에 들어가 트래픽을 받는다. startupProbe는 느린 초기화를 livenessProbe와 분리해 불필요한 재시작을 줄인다.

---

## 4. 트러블슈팅

### KEDA cron scaler가 시간에 맞춰 스케일하지 않음

**증상**: `start` 시간이 지났는데 replica가 증가하지 않음

**원인**: timezone 오타, cron 표현식 오류, KEDA operator 장애, ScaledObject 조건 실패

**해결 방법**:

```bash
kubectl get scaledobject -n default
kubectl describe scaledobject my-app-scheduled-scaler -n default

kubectl get hpa -n default
kubectl describe hpa -n default

kubectl logs -n keda deployment/keda-operator
```

`timezone`은 `Asia/Seoul`처럼 IANA Time Zone Database 값을 사용한다. `start`와 `end`가 같은 값이면 유효하지 않다.

---

### CronJob으로 scale했는데 replica가 다시 줄거나 늘어남

**증상**: CronJob Job은 성공했지만 Deployment replicas가 곧 다른 값으로 바뀜

**원인**: HPA가 같은 Deployment의 replicas를 계속 reconcile함

**해결 방법**:

```bash
kubectl get hpa -n default
kubectl describe hpa my-app-hpa -n default

kubectl get deployment my-app -n default -o jsonpath='{.spec.replicas}'
```

HPA가 있는 워크로드는 CronJob 직접 scale 대신 KEDA cron scaler를 사용한다. 임시로 CronJob 방식을 써야 하면 HPA를 중지하거나, 해당 시간대에 HPA minReplicas를 바꾸는 별도 운영 절차를 둔다.

---

### 스케일 아웃됐지만 Pod가 Pending 상태로 남음

**증상**: replicas는 증가했지만 신규 Pod가 `Pending`이고 트래픽 처리량이 늘지 않음

**원인**: 노드 CPU/Memory 부족, requests 과대 설정, Cluster Autoscaler/Karpenter 반응 지연, node selector/taint 제약

**해결 방법**:

```bash
kubectl get pod -n default -o wide
kubectl describe pod <POD_NAME> -n default
kubectl get events -n default --sort-by=.lastTimestamp
kubectl describe node <NODE_NAME>
```

피크가 예측되면 Pod뿐 아니라 노드 용량도 미리 확보한다. Cluster Autoscaler/Karpenter가 사용하는 node group, provisioner, disruption 정책을 함께 점검한다.

---

### 0개로 줄인 뒤 첫 요청이 느림

**증상**: 야간 scale-to-zero 후 첫 요청에서 timeout 또는 긴 지연 발생

**원인**: 이미지 pull, Pod scheduling, application boot, cache warming 시간이 첫 요청 경로에 포함됨

**해결 방법**:

```bash
kubectl get events -n staging --sort-by=.lastTimestamp
kubectl rollout status deployment/staging-app -n staging
kubectl get pod -n staging -w
```

운영 서비스는 scale-to-zero 대신 최소 1~2개를 유지한다. scale-to-zero가 필요하면 업무 시작 시간보다 충분히 이른 `start`를 설정하고, synthetic check로 warm-up 요청을 보낸다.

---

### scale down이 기대보다 늦음

**증상**: `end` 시간이 지났는데 Pod 수가 바로 줄지 않음

**원인**: KEDA `cooldownPeriod`, HPA `scaleDown.stabilizationWindowSeconds`, PDB, 종료 지연 설정 영향

**해결 방법**:

```bash
kubectl describe scaledobject my-app-scheduled-scaler -n default
kubectl describe hpa -n default
kubectl get pdb -n default
kubectl describe deployment my-app -n default
```

scale down은 서비스 안정성을 위해 의도적으로 지연되는 경우가 많다. 비용 절감이 목적이면 `cooldownPeriod`, HPA behavior, PDB를 함께 조정한다.

---

## 5. 모니터링 및 확인

### 5.1 기본 확인 명령

```bash
# KEDA ScaledObject 상태 확인
kubectl get scaledobject -A
kubectl describe scaledobject my-app-scheduled-scaler -n default

# KEDA가 생성한 HPA 확인
kubectl get hpa -n default
kubectl describe hpa -n default

# Deployment replica와 가용 Pod 확인
kubectl get deployment my-app -n default
kubectl get pod -n default -l app=my-app -o wide

# Pod 리소스 사용량 확인
kubectl top pod -n default
```

### 5.2 CronJob 방식 확인

```bash
kubectl get cronjob -n default
kubectl get job -n default
kubectl logs job/<JOB_NAME> -n default

kubectl auth can-i patch deployments/scale \
  --as=system:serviceaccount:default:scheduled-scaler \
  -n default
```

### 5.3 Prometheus 확인 쿼리

```promql
# Deployment별 ready replica 수
kube_deployment_status_replicas_ready{namespace="default", deployment="my-app"}
```

```promql
# desired replica와 ready replica 차이
kube_deployment_spec_replicas{namespace="default", deployment="my-app"}
-
kube_deployment_status_replicas_ready{namespace="default", deployment="my-app"}
```

```promql
# HPA 현재/최대 replica
kube_horizontalpodautoscaler_status_current_replicas{namespace="default"}
kube_horizontalpodautoscaler_spec_max_replicas{namespace="default"}
```

```promql
# Pod Pending 증가 확인
sum by (namespace) (
  kube_pod_status_phase{phase="Pending"}
)
```

### 5.4 운영 체크리스트

| 항목 | 확인 내용 |
|---|---|
| 피크 시작 전 여유 시간 | 이미지 pull + Pod ready + 노드 확장 시간을 반영 |
| HPA 병행 여부 | CronJob 직접 scale과 충돌하지 않는지 확인 |
| requests 정확도 | HPA CPU 기준과 scheduler 배치 기준이 현실적인지 확인 |
| maxReplicaCount | 예상 최대 트래픽 처리량을 감당하는지 확인 |
| 노드 자동 확장 | Pod 증가량을 받을 node capacity가 있는지 확인 |
| scale down 정책 | cooldown, stabilization, PDB 때문에 비용 절감이 지연되는지 확인 |
| timeZone | UTC 오해 없이 `Asia/Seoul` 등 명시 |

---

## 6. TIP

- 트래픽 피크가 예측되면 HPA만 믿지 말고 피크 시작 10~30분 전에 최소 Pod 수를 올림. 실제 선행 시간은 이미지 크기, node provisioning 시간, 앱 부팅 시간으로 결정함.
- 운영 워크로드는 `minReplicaCount: 0`을 신중히 사용함. 첫 요청 지연과 캐시 미적중이 SLO에 직접 영향을 줌.
- CronJob + `kubectl scale`은 간단하지만 HPA와 충돌하기 쉬움. HPA/KEDA가 있는 환경은 KEDA cron scaler를 우선 사용함.
- `desiredReplicas`는 시간대 내 고정 replica가 아니라 다른 trigger와 함께 계산되는 동적 최소치로 이해함.
- requests 조정은 스케일링 전략의 일부임. requests가 틀리면 HPA 계산과 scheduler 배치가 모두 틀어짐.
- 관련 문서:
  - [HPA/KEDA 오토스케일링](./hpa-keda-autoscaling.md)
  - [리소스 관리](./resource-management.md)
  - [HPA 오브젝트](../objects/hpa.md)
  - [VPA 오브젝트](../objects/vpa.md)
  - [CronJob 오브젝트](../objects/cronjob.md)
  - [공식 문서 - KEDA Cron scaler](https://keda.sh/docs/latest/scalers/cron/)
  - [공식 문서 - Kubernetes CronJob](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/)
  - [공식 문서 - kubectl scale](https://kubernetes.io/docs/reference/kubectl/generated/kubectl_scale/)
