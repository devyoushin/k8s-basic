# Pod 간 통신 (Pod-to-Pod Communication)

## 1. 개요 및 비유

Kubernetes 클러스터 내 모든 Pod가 NAT 없이 서로 직접 통신할 수 있도록 보장하는 네트워킹 모델.

💡 비유: 같은 회사 건물(클러스터) 안의 각 방(Pod)은 내선번호(Pod IP)만 알면 직접 전화할 수 있음. 교환원(NAT) 없이 직통 연결.

---

## 2. 핵심 설명

### 2.1 동작 원리

#### Kubernetes 네트워킹 3원칙

1. **모든 Pod는 고유한 IP를 가짐** — 컨테이너끼리 포트를 공유하지 않아도 됨
2. **Pod IP는 클러스터 전체에서 라우팅 가능** — NAT 변환 없이 직접 통신
3. **노드-Pod 간도 NAT 없이 통신 가능**

#### 케이스별 패킷 경로

**케이스 1: 같은 Pod 내 컨테이너 간 통신**

```
[Container A] ──loopback (lo)──► [Container B]
```

- 동일 네트워크 네임스페이스(Network Namespace) 공유
- `localhost` 또는 `127.0.0.1`로 직접 통신
- 포트 충돌 주의: 같은 Pod 내 컨테이너는 포트를 공유함
- **암호화 불필요**: 호스트 외부로 패킷이 나가지 않음

**케이스 2: 같은 노드의 다른 Pod 간 통신**

```
[Pod A: eth0]
     │
  veth pair
     │
[cbr0 / cni0 브리지]  ← 노드 내 가상 스위치
     │
  veth pair
     │
[Pod B: eth0]
```

- 각 Pod의 `eth0`는 veth(가상 이더넷) pair로 노드 브리지에 연결
- 브리지(cbr0, cni0 등)가 같은 노드 내 Pod 간 L2 스위칭 처리
- 패킷이 커널 내부에서만 이동 → 외부 네트워크 장치 불필요

**케이스 3: 다른 노드의 Pod 간 통신**

```
[Pod A on Node 1]
     │ veth
  [cbr0] → [eth0 Node 1] ──► 언더레이 네트워크 ──► [eth0 Node 2] → [cbr0]
                                                                         │ veth
                                                                    [Pod B on Node 2]
```

- 노드의 물리 NIC(`eth0`)를 통해 외부 네트워크 경유
- CNI 플러그인이 노드 간 라우팅 방식을 결정:

| CNI | 방식 | 오버레이 여부 | 특징 |
|-----|------|-------------|------|
| Flannel (VXLAN) | 터널링 | 있음 | 설정 단순, 오버헤드 있음 |
| Calico (BGP) | 라우팅 | 없음 | 고성능, BGP 라우터 필요 |
| Cilium (eBPF) | eBPF | 선택 | 커널 레벨, 높은 관찰가능성 |
| AWS VPC CNI | VPC 라우팅 | 없음 | Pod IP = VPC IP, EKS 기본 |

#### AWS VPC CNI 동작 방식 (EKS 환경)

```
[Pod A: 10.0.1.5] ──► [ENI Secondary IP: 10.0.1.5] ──► VPC 라우팅 ──► [Pod B: 10.0.2.7]
```

- Pod IP가 VPC 서브넷 IP를 직접 사용 (오버레이 없음)
- 노드 EC2에 여러 ENI를 붙이고, 각 ENI의 Secondary IP를 Pod에 할당
- 노드당 할당 가능한 Pod 수 = ENI 수 × (ENI당 IP 수 - 1)
- VPC 라우팅 테이블이 그대로 적용 → Security Group, Network ACL 효과 있음

#### VXLAN 오버레이 동작 방식 (Flannel 등)

```
[Pod A 패킷]
    │ 원본 패킷: src=10.244.1.5, dst=10.244.2.7
    ▼
[VTEP: VXLAN 캡슐화]
    │ 외부 패킷: src=192.168.1.1(Node1), dst=192.168.1.2(Node2)
    ▼
[노드 간 UDP 전송 (포트 8472)]
    ▼
[VTEP: VXLAN 역캡슐화]
    ▼
[Pod B 수신: 10.244.2.7]
```

---

### 2.2 YAML 적용 예시

#### 같은 Pod 내 컨테이너 간 통신 (sidecar 패턴)

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: multi-container-pod
  namespace: <NAMESPACE>
  labels:
    app: multi-container
    version: v1
spec:
  containers:
    - name: main-app
      image: nginx:1.25
      ports:
        - containerPort: 80
      resources:
        requests:
          memory: "128Mi"
          cpu: "100m"
        limits:
          memory: "256Mi"
          cpu: "500m"
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        readOnlyRootFilesystem: true
        capabilities:
          drop: ["ALL"]

    - name: log-sidecar
      image: busybox:1.36
      # main-app의 80포트에 localhost로 접근 가능
      command: ["sh", "-c", "while true; do wget -qO- localhost:80; sleep 10; done"]
      resources:
        requests:
          memory: "64Mi"
          cpu: "50m"
        limits:
          memory: "128Mi"
          cpu: "100m"
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        readOnlyRootFilesystem: true
        capabilities:
          drop: ["ALL"]
```

#### NetworkPolicy — Pod 간 통신 허용 범위 제한

```yaml
# 기본: 모든 인그레스 차단
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-all-ingress
  namespace: <NAMESPACE>
spec:
  podSelector: {}
  policyTypes:
    - Ingress
---
# payment-svc → order-svc 특정 포트만 허용
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-payment-to-order
  namespace: <NAMESPACE>
spec:
  podSelector:
    matchLabels:
      app: order-svc
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app: payment-svc
      ports:
        - protocol: TCP
          port: 8080
```

#### Istio mTLS — Pod 간 암호화 통신 강제

```yaml
# 네임스페이스 전체 mTLS STRICT 모드
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: default
  namespace: <NAMESPACE>
spec:
  mtls:
    mode: STRICT
---
# SPIFFE ID 기반 서비스 간 접근 제어
apiVersion: security.istio.io/v1beta1
kind: AuthorizationPolicy
metadata:
  name: order-svc-allow
  namespace: <NAMESPACE>
spec:
  selector:
    matchLabels:
      app: order-svc
  action: ALLOW
  rules:
    - from:
        - source:
            principals:
              - "cluster.local/ns/<NAMESPACE>/sa/payment-svc"
      to:
        - operation:
            ports: ["8080"]
```

---

### 2.3 Best Practice

- **NetworkPolicy 기본 차단**: 네임스페이스 생성 시 `deny-all` 정책을 먼저 적용하고 필요한 통신만 허용
- **Pod 간 암호화**: 금융/의료 등 민감 데이터 처리 시 Istio mTLS STRICT 또는 WireGuard(Cilium) 적용
- **같은 Pod 내 컨테이너**: 반드시 강하게 결합된 기능(사이드카, 로그 에이전트)만 배치 — 느슨한 결합은 별도 Pod로 분리
- **AWS VPC CNI 사용 시 IP 부족 주의**: 노드 인스턴스 타입별 ENI/IP 한도 사전 계획 필요
- **서비스 디스커버리**: Pod IP는 재시작마다 변경되므로 직접 사용 금지 — 반드시 Service를 통해 통신

---

## 3. 트러블슈팅

### 3.1 주요 이슈

#### Pod 간 통신 불가 (네트워크 정책 차단)

**증상**: `curl`로 다른 Pod IP 접근 시 타임아웃, `Connection refused` 아님

**원인**: NetworkPolicy가 해당 트래픽을 차단

**해결 방법**:
```bash
# 적용된 NetworkPolicy 확인
kubectl get networkpolicy -n <NAMESPACE>
kubectl describe networkpolicy <POLICY_NAME> -n <NAMESPACE>

# 임시 테스트: 차단 정책 삭제 후 통신 확인
kubectl delete networkpolicy deny-all-ingress -n <NAMESPACE>

# Cilium 사용 시 정책 추적
kubectl exec -n kube-system ds/cilium -- \
  cilium policy trace --src-k8s-pod <NAMESPACE>:<POD_A> \
  --dst-k8s-pod <NAMESPACE>:<POD_B>
```

#### AWS VPC CNI — Pod IP 할당 실패

**증상**: Pod가 `ContainerCreating` 상태로 멈춤, 이벤트에 `Failed to allocate address` 오류

**원인**: 노드의 ENI Secondary IP 한도 초과

**해결 방법**:
```bash
# 노드별 ENI/IP 사용량 확인
kubectl describe node <NODE_NAME> | grep -E "Capacity|Allocatable|eni"

# aws-node DaemonSet 로그에서 IP 할당 오류 확인
kubectl logs -n kube-system ds/aws-node | grep -i "error\|failed"

# WARM_IP_TARGET 조정 (미리 예약해두는 IP 수)
kubectl set env daemonset aws-node -n kube-system WARM_IP_TARGET=2

# 즉시 해결: 노드 추가 또는 더 많은 ENI를 지원하는 인스턴스 타입으로 변경
# 예: m5.large(최대 29 Pod) → m5.xlarge(최대 58 Pod)
```

#### VXLAN 오버레이 MTU 불일치로 패킷 단편화

**증상**: 소용량 패킷은 정상, 대용량 전송(파일 업로드 등) 시 중단

**원인**: VXLAN 헤더(50바이트) 추가로 실제 MTU가 줄어드는데 Pod MTU 설정이 맞지 않음

**해결 방법**:
```bash
# Pod 내부 MTU 확인
kubectl exec <POD_NAME> -n <NAMESPACE> -- ip link show eth0

# 노드 eth0 MTU 확인 (기준값)
ip link show eth0  # 노드에서 실행

# Flannel: MTU 설정 수정
kubectl edit configmap kube-flannel-cfg -n kube-flannel
# net-conf.json 내 "MTU": 1450 으로 조정 (기본 노드 MTU 1500 - VXLAN 오버헤드 50)
```

---

### 3.2 자주 발생하는 문제

#### 같은 Pod 내 컨테이너 포트 충돌

**증상**: Pod가 `Error` 상태, 컨테이너 로그에 `address already in use`

**원인**: 같은 Pod 내 두 컨테이너가 동일 포트 사용

**해결 방법**:
```bash
# 포트 사용 현황 확인
kubectl describe pod <POD_NAME> -n <NAMESPACE> | grep -A 5 "Ports"
# 각 컨테이너 containerPort를 다르게 설정하거나, 하나를 별도 Pod로 분리
```

#### Istio mTLS STRICT 전환 후 헬스체크 실패

**증상**: Pod가 `Running`이지만 Readiness/Liveness probe 실패 반복

**원인**: kubelet 헬스체크 요청이 mTLS를 통과하지 못함

**해결 방법**:
```yaml
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: default
  namespace: <NAMESPACE>
spec:
  mtls:
    mode: STRICT
  portLevelMtls:
    8080:       # 헬스체크 포트만 예외
      mode: DISABLE
```

---

## 4. 모니터링 및 확인

```bash
# Pod IP 및 노드 배치 확인
kubectl get pods -n <NAMESPACE> -o wide

# Pod에서 다른 Pod로 직접 연결 테스트
kubectl exec <POD_NAME> -n <NAMESPACE> -- \
  curl -v http://<TARGET_POD_IP>:<PORT>

# 네트워크 인터페이스 확인 (Pod 내부)
kubectl exec <POD_NAME> -n <NAMESPACE> -- ip addr
kubectl exec <POD_NAME> -n <NAMESPACE> -- ip route

# 노드의 veth pair 확인 (CNI 연결 상태)
# 노드에 SSH 접속 후:
ip link show type veth
brctl show  # 브리지 확인 (net-tools 필요)

# CNI 플러그인 상태 확인
kubectl get pods -n kube-system | grep -E "flannel|calico|cilium|aws-node"
kubectl logs -n kube-system ds/aws-node --tail=50

# Istio mTLS 적용 확인
istioctl authn tls-check <POD_NAME>.<NAMESPACE>

# 네트워크 정책 적용 확인 (Cilium)
kubectl exec -n kube-system ds/cilium -- cilium endpoint list

# tcpdump로 Pod 트래픽 실측 (암호화 여부 확인)
kubectl debug -it <POD_NAME> -n <NAMESPACE> \
  --image=nicolaka/netshoot -- \
  tcpdump -i eth0 -nn port <PORT>
```

---

## 5. TIP

- **Pod IP를 직접 사용하지 말 것**: Pod 재시작 시 IP가 변경됨 → Service DNS (`svc-name.namespace.svc.cluster.local`) 사용
- **같은 노드 배치가 항상 빠르지 않음**: Istio sidecar가 있으면 같은 노드라도 Envoy 프록시를 경유 → 네트워크 홉 자체보다 프록시 오버헤드가 더 큰 경우 있음
- **EKS에서 Pod 수 한도 우회**: `ENABLE_PREFIX_DELEGATION=true` 설정 시 ENI당 /28 prefix 할당 → 노드당 Pod 수 최대 수백 개까지 확장 가능
- **eBPF(Cilium) 도입 시**: kube-proxy 대체 가능 → iptables 규칙 제거로 대규모 클러스터에서 네트워크 성능 향상
- **Ambient Mesh(Istio)**: sidecar 없이 ztunnel이 노드 레벨에서 mTLS 처리 → Pod 재시작 없이 암호화 적용 가능
