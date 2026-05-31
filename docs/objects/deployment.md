## 1. 개요 및 비유
**Deployment(디플로이먼트)**는 파드의 원하는 개수와 버전을 선언하고, 무중단 롤링 업데이트와 롤백을 자동으로 관리해주는 컨트롤러입니다.

💡 **비유하자면 '프랜차이즈 직영점 관리 본부'와 같습니다.**
전국에 항상 매장(Pod) 3개를 유지하라고 지시(replicas: 3)해두면, 한 매장이 문을 닫아도 본부(Deployment)가 즉시 새 매장을 냅니다. 새 메뉴(이미지 버전)로 교체할 때도 모든 매장을 한꺼번에 닫는 대신 하나씩 바꿔가며(Rolling Update) 영업을 유지합니다.

## 2. 핵심 설명
* **ReplicaSet 관리:** Deployment는 직접 파드를 만들지 않고, ReplicaSet을 만들어 파드 개수를 맞춥니다. 버전이 바뀔 때마다 새 ReplicaSet이 생성됩니다.
* **롤링 업데이트 전략:** 새 버전 파드를 하나씩 올리고 이전 버전 파드를 하나씩 내리는 방식으로 무중단 배포를 수행합니다.
  * `maxSurge`: 원하는 복제본 수를 초과해서 한 번에 추가할 수 있는 파드 수
  * `maxUnavailable`: 동시에 서비스 불가 상태가 될 수 있는 파드 수
* **롤백:** `kubectl rollout undo deployment <이름>` 명령어 한 줄로 이전 ReplicaSet으로 즉시 복구합니다.

## 3. YAML 적용 예시 (롤링 업데이트 전략 포함)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app
  namespace: default
spec:
  replicas: 3
  selector:
    matchLabels:
      app: web-app  # 이 라벨로 자신이 관리할 파드를 식별
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1        # 업데이트 중 최대 4개(3+1)까지 파드 허용
      maxUnavailable: 0  # 업데이트 중 서비스 불가 파드 0개 유지 (무중단 보장)
  template:
    metadata:
      labels:
        app: web-app
    spec:
      containers:
      - name: app
        image: my-app:2.0  # 이 값을 변경하면 롤링 업데이트 시작
        ports:
        - containerPort: 8080
        resources:
          requests:
            cpu: "100m"
            memory: "128Mi"
```

**자주 쓰는 명령어:**
```bash
# 새 버전으로 이미지 업데이트 (롤링 업데이트 트리거)
kubectl set image deployment/web-app app=my-app:3.0

# 업데이트 진행 상황 확인
kubectl rollout status deployment/web-app

# 이전 버전으로 롤백
kubectl rollout undo deployment/web-app

# 특정 버전으로 롤백
kubectl rollout undo deployment/web-app --to-revision=2

# 변경 이력 확인
kubectl rollout history deployment/web-app
```

## 4. 트러블 슈팅
* **롤링 업데이트가 멈춘 경우 (`Progressing` 상태에서 멈춤):**
  * 새 파드가 `Pending` 또는 `CrashLoopBackOff`에 빠진 것입니다. `kubectl describe deployment <이름>` 또는 `kubectl get pods`로 새 파드의 상태를 확인하세요.
  * `progressDeadlineSeconds`(기본 600초) 이후 자동으로 실패 처리됩니다.
* **롤백 후에도 문제가 지속:**
  * `kubectl rollout history deployment/<이름>` 으로 버전 이력을 확인하고, `--to-revision` 플래그로 정상이었던 버전을 지정해서 롤백하세요.
* **`maxUnavailable: 0` 설정 시 배포가 느림:**
  * 의도된 동작입니다. 새 파드가 완전히 Ready가 되어야 이전 파드를 내리기 때문입니다. 빠른 배포가 필요하다면 `maxSurge`를 높이세요.
