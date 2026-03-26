## 1. 개요 및 비유
**HPA(HorizontalPodAutoscaler, 수평적 파드 오토스케일러)**는 파드의 CPU, 메모리 사용량 또는 사용자 정의 메트릭을 감시하여 파드의 개수(Replicas)를 자동으로 늘리거나 줄이는 기능입니다.

💡 **비유하자면 '수요에 따라 계산대를 자동으로 열고 닫는 마트 시스템'과 같습니다.**
손님(트래픽)이 몰리면 계산대(파드)를 추가로 열고, 한산해지면 닫는 것처럼 서비스 부하에 따라 파드를 자동으로 조절합니다.

## 2. 핵심 설명
* **동작 방식:** Metrics Server에서 지표 수집 → HPA Controller가 현재 / 목표 메트릭 비율로 필요 레플리카 수 계산 → Deployment/StatefulSet의 Replicas 수정
* **필수 조건:**
  1. 클러스터 내 **Metrics Server** 설치 필수
  2. 대상 파드에 **`resources.requests`** 설정이 반드시 되어 있어야 함 (없으면 `<unknown>` 표시)
* **스케일링 공식:** `필요 레플리카 수 = ceil(현재 레플리카 수 × (현재 메트릭 / 목표 메트릭))`
* **Flapping 방지:** 트래픽이 튀어서 파드가 늘었다 줄었다 반복되는 현상을 막기 위해 `behavior.scaleDown.stabilizationWindowSeconds`(기본 300초) 쿨타임이 있습니다.

## 3. YAML 적용 예시 (autoscaling/v2)

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: web-hpa
  namespace: default
spec:
  scaleTargetRef:              # 스케일링 대상 지정
    apiVersion: apps/v1
    kind: Deployment
    name: web-app
  minReplicas: 2               # 최소 유지 파드 개수
  maxReplicas: 10              # 최대 확장 파드 개수
  metrics:                     # 스케일링 기준 지표
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 60  # CPU 사용량이 평균 60%를 넘으면 스케일 아웃
  - type: Resource
    resource:
      name: memory
      target:
        type: AverageValue
        averageValue: 500Mi     # 메모리 평균 사용량이 500Mi를 넘으면 스케일 아웃
  behavior:                    # 상세 스케일링 속도 조절
    scaleDown:
      stabilizationWindowSeconds: 300  # 스케일 인 전 5분 대기 (Flapping 방지)
      policies:
      - type: Percent
        value: 100
        periodSeconds: 15
    scaleUp:
      stabilizationWindowSeconds: 0   # 스케일 아웃은 즉시
      policies:
      - type: Pods
        value: 4              # 한 번에 최대 4개까지 추가
        periodSeconds: 15
```

## 4. 명령어 결과 분석 (kubectl get hpa)

```
$ kubectl get hpa

NAME      REFERENCE            TARGETS          MINPODS   MAXPODS   REPLICAS   AGE
web-hpa   Deployment/web-app   45%/60%, 12%/70%   2         10        3          15m
```

* **TARGETS:** `현재지표 / 목표치` — `45%/60%`는 현재 평균 사용량 45%, 목표 60%
* **`<unknown>/60%` 출력 시:** Metrics Server가 지표를 못 가져오는 상태 (파드에 `requests` 미설정 확인)
* **REPLICAS:** 현재 실제로 떠 있는 파드 개수

## 5. 트러블 슈팅
* **HPA가 스케일링을 하지 않음:**
  * `kubectl describe hpa <이름>` 에서 `Conditions` 섹션을 확인하세요. `ScalingActive: False` 이면 Metrics Server 문제입니다.
  * 파드에 `resources.requests` 가 설정되어 있는지 확인하세요.
* **Flapping(진동) 현상 발생:**
  * 파드가 늘어났다 줄었다를 반복합니다. `behavior.scaleDown.stabilizationWindowSeconds` 값을 300~600으로 늘려 보수적으로 스케일 인하도록 설정하세요.
* **수동 Replica 수정 금지:**
  * HPA가 설정된 리소스는 `kubectl scale`로 수동 수정해도 HPA가 다시 원래대로 되돌립니다.
