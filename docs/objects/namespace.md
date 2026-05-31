## 1. 개요 및 비유
**Namespace(네임스페이스)**는 하나의 물리적 쿠버네티스 클러스터 안에서 리소스들을 논리적으로 격리하는 가상 파티션입니다.

💡 **비유하자면 '아파트 동(棟)'과 같습니다.**
한 단지(클러스터) 안에 101동(dev), 102동(staging), 103동(prod) 이 있다고 생각하세요. 각 동은 같은 주차장(물리 노드)을 쓰지만, 주민 명단(리소스)은 동별로 따로 관리됩니다. 103동 사람(파드)이 101동 누구에게 전화(통신) 하려면 동 번호까지 포함한 주소를 써야 합니다.

## 2. 핵심 설명
* **기본 네임스페이스:**
  * `default`: 아무 네임스페이스를 지정하지 않으면 여기에 생성됩니다.
  * `kube-system`: API Server, etcd, Scheduler 등 쿠버네티스 시스템 컴포넌트가 위치합니다.
  * `kube-public`: 인증 없이 모든 사용자가 읽을 수 있는 공개 데이터 (주로 cluster-info).
  * `kube-node-lease`: 노드 하트비트용 Lease 오브젝트 저장 공간.
* **Namespace 범위 리소스 vs 클러스터 범위 리소스:**
  * Namespace 내: Pod, Deployment, Service, ConfigMap, Secret 등
  * 클러스터 전체: Node, PersistentVolume, ClusterRole, Namespace 자체
* **DNS 격리:** 같은 네임스페이스 내에서는 서비스 이름만으로 통신 가능 (`backend-svc`). 다른 네임스페이스의 서비스에 접근하려면 FQDN을 사용해야 합니다 (`backend-svc.production.svc.cluster.local`).
* **ResourceQuota & LimitRange:** 네임스페이스 단위로 CPU/메모리 총량을 제한하거나 파드별 기본값을 설정할 수 있습니다.

## 3. YAML 적용 예시

### Namespace 생성
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: production
  labels:
    env: prod
    team: backend
```

### ResourceQuota (네임스페이스 전체 자원 제한)
```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: prod-quota
  namespace: production
spec:
  hard:
    requests.cpu: "10"        # 네임스페이스 내 모든 파드의 CPU 요청 합계 상한
    requests.memory: "20Gi"
    limits.cpu: "20"
    limits.memory: "40Gi"
    count/pods: "100"         # 최대 파드 개수
    count/services: "20"
```

### LimitRange (파드별 기본값 및 상/하한 설정)
```yaml
apiVersion: v1
kind: LimitRange
metadata:
  name: default-limits
  namespace: production
spec:
  limits:
  - type: Container
    default:            # requests/limits 미설정 시 자동 부여
      cpu: "200m"
      memory: "256Mi"
    defaultRequest:
      cpu: "100m"
      memory: "128Mi"
    max:                # 이 값을 초과하는 파드는 거부됨
      cpu: "2"
      memory: "2Gi"
```

**유용한 명령어:**
```bash
# 특정 네임스페이스의 모든 리소스 조회
kubectl get all -n production

# 전체 네임스페이스에서 파드 조회
kubectl get pods -A

# 현재 컨텍스트의 기본 네임스페이스 변경
kubectl config set-context --current --namespace=production
```

## 4. 트러블 슈팅
* **다른 네임스페이스의 서비스에 연결 안 됨:**
  * `<서비스명>.<네임스페이스>.svc.cluster.local` 형식의 FQDN을 사용하세요.
  * NetworkPolicy가 크로스 네임스페이스 트래픽을 차단하고 있는지도 확인하세요.
* **리소스 생성 시 `exceeded quota` 에러:**
  * `kubectl describe resourcequota -n <네임스페이스>` 로 현재 사용량과 제한을 비교하세요.
  * 남은 쿼터 내에서 리소스의 `requests` 값을 줄이거나 쿼터를 늘려야 합니다.
* **네임스페이스 삭제가 `Terminating` 상태에서 멈춤:**
  * 네임스페이스 내 finalizer가 걸린 리소스가 있거나, CRD 컨트롤러가 죽어 있는 경우입니다. `kubectl get all -n <네임스페이스>` 로 잔류 리소스를 확인하고 수동 삭제하세요.
