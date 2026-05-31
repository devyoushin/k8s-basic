## 1. 개요 및 비유
**Service(서비스)**는 파드들에게 안정적인 네트워크 접근 주소를 제공하는 추상화 레이어입니다. 파드는 언제든 교체될 수 있어 IP가 바뀌지만, Service는 변하지 않는 단일 진입점(고정 IP/DNS)을 제공합니다.

💡 **비유하자면 '대표 전화번호'와 같습니다.**
콜센터 직원(Pod)이 자리를 바꾸거나 교체되어도, 고객은 항상 같은 대표번호(Service)로 연결됩니다. 교환기(kube-proxy)가 현재 일하고 있는 직원에게 자동으로 연결해줍니다.

## 2. 핵심 설명

### 서비스 타입 비교

| 타입 | 접근 범위 | 사용 사례 |
|---|---|---|
| **ClusterIP** (기본값) | 클러스터 내부에서만 접근 가능 | 내부 마이크로서비스 간 통신 |
| **NodePort** | 모든 노드의 특정 포트로 외부 접근 가능 | 개발/테스트 환경, 간단한 외부 노출 |
| **LoadBalancer** | 클라우드 로드밸런서 생성 후 외부 접근 | 프로덕션 외부 서비스 노출 |
| **ExternalName** | 클러스터 내부에서 외부 DNS를 별칭으로 접근 | 외부 DB/API를 내부 서비스처럼 사용 |

* **Selector 기반 라우팅:** `selector` 필드의 라벨과 일치하는 파드들에게 트래픽을 분산합니다. 파드가 추가/삭제되면 자동으로 엔드포인트가 업데이트됩니다.
* **DNS 자동 등록:** 서비스가 생성되면 CoreDNS가 `<서비스명>.<네임스페이스>.svc.cluster.local` 형태의 DNS를 자동 등록합니다.

## 3. YAML 적용 예시

### ClusterIP (내부 통신용)
```yaml
apiVersion: v1
kind: Service
metadata:
  name: backend-svc
  namespace: default
spec:
  type: ClusterIP  # 생략해도 기본값
  selector:
    app: backend   # 이 라벨을 가진 파드들에게 트래픽 전달
  ports:
  - protocol: TCP
    port: 80       # 서비스가 받는 포트
    targetPort: 8080  # 파드가 실제로 리스닝하는 포트
```

### NodePort (개발/테스트용 외부 노출)
```yaml
apiVersion: v1
kind: Service
metadata:
  name: web-nodeport
spec:
  type: NodePort
  selector:
    app: web
  ports:
  - port: 80
    targetPort: 8080
    nodePort: 30080  # 30000~32767 범위. 생략 시 자동 할당
```
> 접근 방법: `http://<노드IP>:30080`

### LoadBalancer (클라우드 프로덕션 환경)
```yaml
apiVersion: v1
kind: Service
metadata:
  name: web-lb
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"  # AWS NLB 사용
spec:
  type: LoadBalancer
  selector:
    app: web
  ports:
  - port: 443
    targetPort: 8443
```

**유용한 명령어:**
```bash
# 서비스 엔드포인트(실제 파드 IP 목록) 확인
kubectl get endpoints backend-svc

# 서비스 상세 정보 확인 (LB IP, 포트 등)
kubectl describe service web-lb
```

## 4. 트러블 슈팅
* **서비스로 접근이 안 될 때:**
  1. `kubectl get endpoints <서비스명>` 실행 후 `<none>`이면 selector 라벨이 파드 라벨과 불일치합니다.
  2. 파드의 `containerPort`와 서비스의 `targetPort`가 일치하는지 확인하세요.
  3. `kubectl exec`로 파드 내부에서 `curl localhost:<targetPort>`가 되는지 먼저 확인하세요.
* **LoadBalancer의 EXTERNAL-IP가 `<pending>`에서 안 바뀜:**
  * 클라우드 환경이 아니거나, IAM/노드 권한 문제입니다. 온프레미스에서는 MetalLB 같은 별도 솔루션이 필요합니다.
* **ClusterIP로만 접근 가능한 서비스를 로컬에서 테스트하려면:**
  * `kubectl port-forward service/<서비스명> 8080:80` 으로 로컬 포트를 터널링하세요.
