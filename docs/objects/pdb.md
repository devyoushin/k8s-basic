## 1. 개요 및 비유
**PodDisruptionBudget(PDB)**은 자발적 중단(Voluntary Disruption) 상황에서 서비스가 유지되어야 할 최소 파드 수 또는 최대 중단 가능 파드 수를 보장하는 정책입니다.

💡 **비유하자면 '비행기 최소 승무원 규정'과 같습니다.**
항공사가 비용 절감을 위해 기내 직원을 줄이더라도, 항공법상 최소 승무원 수(minAvailable)는 반드시 유지해야 합니다. 노드 드레인, 클러스터 업그레이드 같은 계획된 작업 시 쿠버네티스가 이 규정을 지켜서 서비스가 중단되지 않도록 합니다.

## 2. 핵심 설명

### 자발적 중단 vs 비자발적 중단

| 구분 | 예시 | PDB 적용 여부 |
|---|---|---|
| **자발적 중단** (Voluntary) | 노드 드레인, 클러스터 업그레이드, 스케일 다운 | ✅ PDB가 보호 |
| **비자발적 중단** (Involuntary) | 노드 하드웨어 장애, OOM 킬, 커널 패닉 | ❌ PDB로 보호 불가 |

### minAvailable vs maxUnavailable

| 설정 | 의미 | 사용 시점 |
|---|---|---|
| `minAvailable` | 항상 최소 N개(또는 N%)는 Running 상태 유지 | 최소 가용성이 중요할 때 |
| `maxUnavailable` | 동시에 최대 N개(또는 N%)까지만 중단 허용 | 중단 속도를 제한할 때 |

둘 중 하나만 설정합니다. 퍼센트(%) 또는 절대값 모두 지원합니다.

### PDB가 적용되는 상황
- `kubectl drain <노드>` — 노드 점검 전 파드 이동
- 클러스터 업그레이드 (관리형 서비스의 노드 롤링 업데이트)
- Cluster Autoscaler의 노드 스케일 다운
- `kubectl delete pod` (직접 삭제도 자발적 중단으로 간주)

## 3. YAML 적용 예시

### minAvailable (절대값)
```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: web-pdb
  namespace: production
spec:
  minAvailable: 2          # 항상 최소 2개 파드는 Running 유지
  selector:
    matchLabels:
      app: web             # 이 라벨의 파드에 적용
```
> Deployment replicas가 3이면 한 번에 최대 1개만 중단 가능.

### maxUnavailable (퍼센트)
```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: api-pdb
  namespace: production
spec:
  maxUnavailable: 25%      # 전체의 25% 이하만 동시 중단 허용
  selector:
    matchLabels:
      app: api
```
> replicas가 8이면 최대 2개까지 동시 중단 허용 (6개는 항상 유지).

### StatefulSet과 함께 사용 (DB 클러스터)
```yaml
# MySQL 3개 중 최소 2개(리더 + 팔로워 1개)는 항상 유지
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: mysql-pdb
  namespace: production
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: mysql
```

### 현재 PDB 상태 확인
```bash
# PDB 상태 조회
kubectl get pdb -n production

# 출력 예시:
# NAME       MIN AVAILABLE   MAX UNAVAILABLE   ALLOWED DISRUPTIONS   AGE
# web-pdb    2               N/A               1                     5d
# api-pdb    N/A             25%               2                     5d

# ALLOWED DISRUPTIONS: 현재 추가로 중단 가능한 파드 수
# 0이면 지금 drain하면 PDB 위반 → 블로킹됨
```

**노드 드레인 시 PDB 확인:**
```bash
# drain 시도 시 PDB 위반이면 블로킹됨
kubectl drain node-1 --ignore-daemonsets --delete-emptydir-data

# 강제로 PDB 무시하고 drain (장애 상황 등 불가피한 경우만)
kubectl drain node-1 --ignore-daemonsets --delete-emptydir-data --disable-eviction
```

## 4. 트러블 슈팅

* **노드 drain이 무한정 멈추고 완료가 안 됨:**
  * PDB의 `minAvailable` 조건을 충족할 수 없어서 블로킹된 것입니다.
  ```bash
  # 어떤 PDB가 drain을 막고 있는지 확인
  kubectl get pdb -A
  # ALLOWED DISRUPTIONS가 0인 PDB 찾기

  kubectl describe pdb <pdb명> -n <네임스페이스>
  # "Cannot evict pod as it would violate the pod's disruption budget" 확인
  ```
  * 해결: 해당 앱의 replicas를 임시로 늘리거나, 다른 노드에 파드가 먼저 배치된 후 drain을 재시도하세요.

* **`ALLOWED DISRUPTIONS`가 계속 0인 경우:**
  * 현재 파드 수가 이미 `minAvailable`과 같거나 그 이하인 것입니다.
  * `kubectl get pods -l app=<앱명>` 으로 Running 상태인 파드 수를 확인하고, replicas를 늘리거나 PDB 값을 조정하세요.

* **PDB를 설정했는데 Cluster Autoscaler가 노드를 못 줄임:**
  * 의도된 동작입니다. CA는 PDB를 존중하여 스케일 다운 시 PDB 위반 여부를 확인합니다.
  * 파드를 다른 노드로 이동 가능한 상태인지 확인하세요. (리소스 여유, Affinity 조건 등)

* **replicas: 1인 Deployment에 PDB를 걸면 drain이 영원히 불가:**
  * `minAvailable: 1`이면 1개가 항상 필요한데, drain하면 0개가 되므로 불가합니다.
  * 싱글 레플리카 앱은 PDB를 걸기 전에 반드시 HA(replicas ≥ 2)로 먼저 전환하세요.
  * 또는 `minAvailable: 0`으로 설정하면 형식상 PDB는 있지만 drain은 허용됩니다.
