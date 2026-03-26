## 1. 개요 및 비유
**Scheduler**는 새로 생성된 파드를 어떤 노드에 배치할지 결정하는 '배정 담당자'입니다. 

💡 **비유하자면 '대학교 수강신청 시스템 또는 기숙사 배정 담당자'와 같습니다.**
학생(Pod)이 들어오면, 각 건물(Node)의 빈자리 현황, 학생의 전공(필요 자원), 선호하는 방 친구(Affinity) 등을 고려해서 최적의 방에 배정해 줍니다. "이 학생은 휠체어를 타니까 엘리베이터가 있는 건물(GPU 노드 등)에 배치해야 해" 같은 복잡한 조건도 처리합니다.



## 2. 핵심 설명
* **2단계 결정 과정:** 1. **Filtering:** 파드가 요구하는 자원(CPU/MEM)과 조건(Selector)을 만족하지 못하는 노드를 후보에서 제외합니다.
  2. **Scoring:** 남은 후보 노드 중 가장 점수가 높은(자원이 넉넉하거나 네트워크가 가까운 등) 노드를 최종 선택합니다.
* **오직 배치 결정만:** 스케줄러는 "어디에 둘지"만 결정할 뿐, 실제로 파드를 띄우는 건 Kubelet의 몫입니다.

## 3. YAML 적용 예시 (Node Affinity 설정)
스케줄러에게 특정 라벨이 붙은 노드에만 파드를 배치하도록 지시하는 설정입니다.

```yaml
spec:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: disktype
            operator: In
            values:
            - ssd # disktype=ssd 라벨이 있는 노드에만 배치해라!
```

## 4. 트러블 슈팅
* **파드가 계속 `Pending` 상태인 경우:**
  * `kubectl describe pod` 명령어를 쳤을 때 `0/3 nodes are available: 3 Insufficient cpu` 같은 메시지가 있다면 스케줄러가 파드를 보낼 빈자리를 찾지 못한 것입니다. 노드를 추가하거나 파드의 요구 자원을 줄여야 합니다.
