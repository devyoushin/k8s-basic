### 1. 개요
HPA는 Pod의 CPU, Memory 사용량 또는 사용자 정의 메트릭(Custom Metrics)을 감시하여, 설정된 목표치에 도달하도록 **Pod의 개수(Replicas)를 자동으로 늘리거나 줄이는** 기능입니다.

* **동작 방식**: Metrics Server에서 지표 수집 → HPA Controller가 계산 → Deployment/StatefulSet의 Replicas 수정.
* **필수 조건**: 
    1. 클러스터 내 **Metrics Server** 설치 필수.
    2. 대상 Pod에 **`resources.requests`** 설정이 반드시 되어 있어야 함.

---

### 2. 표준 YAML 설정 예시 (autoscaling/v2)

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: web-hpa
  namespace: default
spec:
  scaleTargetRef:         # 스케일링 대상 지정
    apiVersion: apps/v1
    kind: Deployment
    name: web-app
  minReplicas: 2          # 최소 유지 Pod 개수
  maxReplicas: 10         # 최대 확장 Pod 개수
  metrics:                # 스케일링 기준 지표
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
        averageValue: 500Mi      # 메모리 평균 사용량이 500Mi를 넘으면 스케일 아웃
  behavior:               # (선택) 상세 스케일링 속도 조절
    scaleDown:
      stabilizationWindowSeconds: 300 # 갑작스러운 축소를 방지하기 위해 5분간 대기
      policies:
      - type: Percent
        value: 100        # 한 번에 줄일 수 있는 최대치(%)
        periodSeconds: 15        
```

### 3. 주요 설정 파라미터 상세 설명

|**항목**|**상세 설명**|
|---|---|
|**scaleTargetRef**|어떤 객체를 스케일링할지 결정 (Deployment, StatefulSet 등).|
|**minReplicas / maxReplicas**|트래픽이 없어도 최소 N개를 유지하고, 아무리 많아도 M개를 넘지 않도록 제한함.|
|**target.type**|`Utilization`(백분율), `AverageValue`(절대값) 중 선택.|
|**averageUtilization**|요청량(Request) 대비 사용량의 비율. (Request가 100m인데 사용량이 60m이면 60%)|
|**behavior**|급격한 스케일링(Thrashing)을 방지하는 정책. `stabilizationWindowSeconds`는 확장이 끝난 후 축소하기까지 기다리는 일종의 '쿨타임'임.|

---

### 4. 명령어 결과 분석 (kubectl get hpa)

명령어 실행 시 출력되는 각 컬럼의 의미입니다.

```
$ kubectl get hpa

NAME      REFERENCE            TARGETS         MINPODS   MAXPODS   REPLICAS   AGE
web-hpa   Deployment/web-app   45%/60%, 12%/70%  2         10        3          15m
```

1. **REFERENCE**: HPA가 관리하고 있는 대상 리소스(Deployment 이름 등).
2. **TARGETS**: `현재지표 / 목표치`.
    
    - 예: `45%/60%`는 현재 평균 사용량이 45%이고 목표가 60%임을 의미.
        
    - **`<unknown>/60%` 출력 시**: Metrics Server가 지표를 가져오지 못하는 상태. (Pod에 `requests`가 설정되었는지 확인 필요)
        
3. **MINPODS / MAXPODS**: 설정된 최소/최대 복제본 수.
4. **REPLICAS**: 현재 실제로 떠 있는 Pod의 개수.
    

---

### 5. 실무 활용 팁 (Troubleshooting)

- **Pod에 리소스 제한(Requests)이 없으면 동작하지 않음**: HPA는 Request 값을 기준으로 %를 계산하기 때문입니다.
    
- **Flapping(진동) 현상 주의**: 트래픽이 튀어서 Pod가 늘어났다가 바로 줄어드는 현상이 반복되면 서비스 안정성이 떨어집니다. 이 경우 `behavior.scaleDown.stabilizationWindowSeconds` 값을 높여(예: 300~600) 보수적으로 축소하게 설정합니다.
    
- **수동 Replica 수정 금지**: HPA가 설정된 리소스의 복제본 수는 HPA가 제어하게 두어야 합니다. `kubectl scale`로 수동 수정해도 HPA가 다시 원래대로 되돌려버립니다.
