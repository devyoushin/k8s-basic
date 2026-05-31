## 1. 개요 및 비유
**DaemonSet(데몬셋)**은 클러스터 내의 모든 워커 노드(또는 특정 노드)마다 파드를 정확히 한 개씩만 띄우도록 보장하는 컨트롤러입니다.

💡 **비유하자면 '각 층마다 배치된 경비원(CCTV)'과 같습니다.**
건물(클러스터)에 새로운 층(워커 노드)이 증축되면, 관리자가 지시하지 않아도 데몬셋 시스템이 알아서 새 층에 경비원(파드)을 1명 배치합니다. 반대로 층이 철거되면 경비원도 자동으로 철수하죠. 주로 모든 노드에서 돌아가야 하는 **로그 수집기, 모니터링 에이전트, 네트워크(CNI) 플러그인**에 사용됩니다.

[Image of Kubernetes DaemonSet ensuring one pod runs on every node]

## 2. 핵심 설명
* **자동 스케일링 동기화:** HPA처럼 CPU에 따라 파드 개수를 조절하는 것이 아니라, 클러스터에 워커 노드가 추가되면 그 개수에 비례하여 파드도 하나씩 늘어나는 방식입니다.
* **Tolerations (용인):** 일반적으로 마스터 노드나 특정 작업용 노드에는 파드가 스케줄링되지 않도록 Taint(오점)가 걸려 있습니다. 하지만 데몬셋은 주로 인프라 레벨의 필수 파드이므로, 이 Taint를 무시하고 무조건 들어갈 수 있도록 Toleration 설정이 포함되는 경우가 많습니다.

## 3. YAML 적용 예시 (로그 수집기 배치)
모든 노드의 로그를 수집하기 위해 Fluent Bit 파드를 각 노드마다 1개씩 배포하는 데몬셋 예시입니다.

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
      containers:
      - name: fluent-bit
        image: fluent/fluent-bit:latest
        volumeMounts:
        - name: varlog # 노드의 실제 로그 경로를 마운트
          mountPath: /var/log
      volumes:
      - name: varlog
        hostPath:
          path: /var/log
```

## 4. 트러블 슈팅
* **새로 추가된 노드에 데몬셋 파드가 뜨지 않음:**
  * 새 노드에 카펜터(Karpenter)나 관리자가 특정 Taint(예: `dedicated=gpu:NoSchedule`)를 걸어두었을 확률이 높습니다. 데몬셋 YAML 스펙에 해당 Taint를 무시하는 `tolerations` 구문이 누락되었는지 확인하세요.
* **데몬셋 롤링 업데이트 지연:**
  * 기본적으로 데몬셋은 한 번에 하나씩(또는 설정된 `maxUnavailable` 비율만큼) 파드를 교체합니다. 노드가 100개라면 업데이트에 매우 오랜 시간이 걸릴 수 있으므로, 전략 설정을 적절히 튜닝해야 합니다.
