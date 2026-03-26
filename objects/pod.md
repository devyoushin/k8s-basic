## 1. 개요 및 비유
**Pod(파드)**는 쿠버네티스에서 배포할 수 있는 가장 작은 단위입니다. 하나 이상의 컨테이너를 함께 묶어서 동일한 네트워크 네임스페이스와 스토리지를 공유하게 만든 단위입니다.

💡 **비유하자면 '룸메이트가 있는 기숙사 방'과 같습니다.**
방(Pod) 안에 사는 룸메이트들(컨테이너)은 같은 주소(IP), 같은 로컬 폰(localhost), 같은 책상 서랍(Volume)을 공유합니다. 혼자 사는 방(단일 컨테이너)이 가장 흔하지만, 메인 앱 옆에 사이드카 컨테이너(로그 수집기, 프록시 등)를 같이 두는 경우도 많습니다.

## 2. 핵심 설명
* **IP 1개 공유:** 파드 안의 모든 컨테이너는 동일한 IP를 갖습니다. 컨테이너끼리 `localhost`로 통신 가능합니다.
* **일시성(Ephemeral):** 파드는 죽으면 재시작되지 않고 새로운 파드로 교체됩니다. IP도 바뀝니다. 직접 파드를 운영하는 대신 Deployment/StatefulSet 같은 컨트롤러를 사용해야 하는 이유입니다.
* **Probe(상태 검사):** Kubelet이 컨테이너 상태를 주기적으로 검사합니다.
  * `livenessProbe`: 컨테이너가 살아있는지 확인 (실패 시 재시작)
  * `readinessProbe`: 트래픽을 받을 준비가 되었는지 확인 (실패 시 Service 엔드포인트에서 제외)
  * `startupProbe`: 시작이 느린 앱을 위한 초기화 완료 대기 (완료 전까지 다른 Probe 비활성화)
* **리소스 요청/제한:** `requests`는 스케줄링 기준, `limits`는 실제 사용 상한선입니다.

## 3. YAML 적용 예시 (Probe 및 리소스 설정)

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: web-server
  labels:
    app: web
spec:
  containers:
  - name: nginx
    image: nginx:1.25
    ports:
    - containerPort: 80
    resources:
      requests:
        cpu: "100m"      # 스케줄러가 이 값을 기준으로 노드 배치
        memory: "128Mi"
      limits:
        cpu: "500m"      # 이 값을 초과하면 CPU 쓰로틀링 발생
        memory: "256Mi"  # 이 값을 초과하면 OOMKilled로 재시작
    livenessProbe:
      httpGet:
        path: /healthz
        port: 80
      initialDelaySeconds: 10  # 컨테이너 시작 후 10초 뒤부터 검사
      periodSeconds: 5         # 5초마다 검사
      failureThreshold: 3      # 3번 연속 실패 시 재시작
    readinessProbe:
      httpGet:
        path: /ready
        port: 80
      initialDelaySeconds: 5
      periodSeconds: 5
```

## 4. 트러블 슈팅
* **파드가 `CrashLoopBackOff` 상태:**
  * 컨테이너가 계속 죽어서 재시작되는 상태입니다. `kubectl logs <파드명> --previous` 로 이전 컨테이너 로그를 확인하세요.
  * `livenessProbe` 설정이 너무 공격적이거나, 앱 내부 오류일 수 있습니다.
* **파드가 `OOMKilled` 상태:**
  * `limits.memory`를 초과하여 커널이 강제 종료한 것입니다. `kubectl describe pod <파드명>`에서 `OOMKilled` 확인 후 메모리 제한을 늘리거나 앱의 메모리 누수를 찾으세요.
* **`ImagePullBackOff` 에러:**
  * 컨테이너 이미지를 받지 못한 것입니다. 이미지 이름/태그 오타, 프라이빗 레지스트리 접근 시 `imagePullSecrets` 설정 누락 여부를 확인하세요.
