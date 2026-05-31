# eBPF 기반 CNI — Cilium

## 1. 개요 및 비유

Cilium은 Linux 커널의 eBPF(extended Berkeley Packet Filter) 기술을 활용해 kube-proxy 없이 고성능 네트워킹, 보안 정책, 관찰가능성을 제공하는 CNI 플러그인.

💡 비유: 기존 iptables 방식이 모든 택배를 창고(iptables 규칙 체인)를 거쳐 분류하는 것이라면, eBPF는 택배 트럭이 출발하는 순간 목적지를 바로 계산해 창고를 우회하는 것. 창고(체인) 길이에 무관하게 일정한 속도 유지.

---

## 2. 핵심 설명

### 2.1 동작 원리

#### eBPF란

- Linux 커널 내에서 실행되는 샌드박스 프로그램
- 커널 수정이나 모듈 로드 없이 네트워크 패킷, 시스템 콜, 트레이싱 처리 가능
- JIT 컴파일로 네이티브 코드 수준 성능
- 검증기(Verifier)가 안전성 보장 (무한루프, 메모리 침범 방지)

#### iptables vs eBPF 성능 비교

| 항목 | iptables (kube-proxy) | eBPF (Cilium) |
|------|-----------------------|---------------|
| 규칙 처리 방식 | 선형 체인 순회 O(n) | 해시맵 조회 O(1) |
| 서비스 1만 개 기준 지연 | ~ms 단위 증가 | 거의 무변화 |
| 규칙 업데이트 | 전체 테이블 재작성 | 점진적 업데이트 |
| 커널 패킷 우회 | 불가 | XDP로 드라이버 레벨 처리 |

#### Cilium 핵심 기능

```
[Cilium 기능 스택]
  ├── 네트워킹: CNI, kube-proxy 대체, BGP, VXLAN/Geneve 오버레이
  ├── 보안: NetworkPolicy (L3/L4), CiliumNetworkPolicy (L7 HTTP/gRPC/DNS)
  ├── 암호화: WireGuard 커널 레벨 Pod-to-Pod 암호화
  ├── 로드밸런싱: DSR (Direct Server Return), Maglev 해싱
  └── 관찰가능성: Hubble (흐름 가시성), Prometheus 메트릭
```

#### Cilium + WireGuard Pod 간 암호화

```
[Node 1]                              [Node 2]
  Pod A ──► Cilium eBPF ──► WireGuard 터널 ──► Cilium eBPF ──► Pod B
                              (커널 레벨 암호화)
```

- Istio mTLS와 달리 애플리케이션/사이드카 개입 없이 L3에서 암호화
- CPU 오버헤드: Istio sidecar 대비 낮음 (커널 구현)
- 단점: L7 정책(HTTP 메서드, Path 기반) 불가 → L7 제어가 필요하면 Istio 병용

#### Hubble — 네트워크 흐름 가시성

```
[eBPF Hook] → [Hubble Agent] → [Hubble Relay] → [Hubble UI / CLI]
```

- 모든 네트워크 흐름을 드롭 없이 관찰 (패킷 샘플링 아님)
- L7 프로토콜(HTTP, DNS, gRPC) 요청/응답 자동 파싱
- NetworkPolicy 차단 이벤트 실시간 확인

---

### 2.2 YAML 적용 예시

#### Cilium 설치 (Helm, kube-proxy 대체 모드)

```bash
helm repo add cilium https://helm.cilium.io/

helm install cilium cilium/cilium \
  --namespace kube-system \
  --set kubeProxyReplacement=true \
  --set k8sServiceHost=<API_SERVER_IP> \
  --set k8sServicePort=6443 \
  --set encryption.enabled=true \
  --set encryption.type=wireguard \
  --set hubble.relay.enabled=true \
  --set hubble.ui.enabled=true
```

#### CiliumNetworkPolicy — L7 HTTP 정책 (Kubernetes NetworkPolicy 확장)

```yaml
# L7 수준: HTTP 메서드와 경로까지 제어
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
metadata:
  name: payment-api-policy
  namespace: payments
spec:
  endpointSelector:
    matchLabels:
      app: payment-svc
  ingress:
    - fromEndpoints:
        - matchLabels:
            app: api-gateway
      toPorts:
        - ports:
            - port: "8080"
              protocol: TCP
          rules:
            http:
              - method: "POST"
                path: "/api/v1/payments"
              - method: "GET"
                path: "/api/v1/payments/[0-9]+"
  egress:
    - toFQDNs:
        - matchPattern: "*.redis.cache.amazonaws.com"
      toPorts:
        - ports:
            - port: "6380"
              protocol: TCP
```

#### CiliumNetworkPolicy — DNS 기반 Egress 제어

```yaml
# 특정 외부 도메인만 허용
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
metadata:
  name: allow-external-apis
  namespace: payments
spec:
  endpointSelector:
    matchLabels:
      app: payment-svc
  egress:
    # DNS 조회 허용 (CoreDNS)
    - toEndpoints:
        - matchLabels:
            k8s:io.kubernetes.pod.namespace: kube-system
            k8s:k8s-app: kube-dns
      toPorts:
        - ports:
            - port: "53"
              protocol: UDP
          rules:
            dns:
              - matchPattern: "*"
    # 허용된 외부 도메인
    - toFQDNs:
        - matchName: "api.visa.com"
        - matchName: "api.mastercard.com"
      toPorts:
        - ports:
            - port: "443"
              protocol: TCP
```

#### WireGuard 암호화 활성화 확인

```bash
# WireGuard 인터페이스 확인 (노드에서)
ip link show cilium_wg0

# Cilium 암호화 상태
kubectl exec -n kube-system ds/cilium -- cilium encrypt status
```

#### Hubble CLI — 실시간 흐름 모니터링

```bash
# Hubble CLI 설치
export HUBBLE_VERSION=$(curl -s https://raw.githubusercontent.com/cilium/hubble/master/stable.txt)
curl -L --remote-name-all \
  "https://github.com/cilium/hubble/releases/download/$HUBBLE_VERSION/hubble-linux-amd64.tar.gz"
tar xzvf hubble-linux-amd64.tar.gz

# Hubble Relay 포트 포워딩
kubectl port-forward -n kube-system svc/hubble-relay 4245:80 &

# 특정 네임스페이스 흐름 실시간 조회
hubble observe --namespace payments --follow

# NetworkPolicy 차단 이벤트만 필터링
hubble observe --namespace payments --verdict DROPPED --follow

# L7 HTTP 흐름 조회
hubble observe --namespace payments --protocol http --follow
```

---

### 2.3 Best Practice

- **kube-proxy 완전 대체**: `kubeProxyReplacement=true` 설정으로 iptables 규칙 제거 → 대규모 클러스터에서 서비스 업데이트 속도 대폭 향상
- **WireGuard 암호화**: 금융권 Pod-to-Pod 암호화에서 Istio mTLS 대신 또는 병용으로 사용 — L7 정책이 불필요한 백엔드 간 통신에 유리
- **Hubble 필수 활성화**: NetworkPolicy 디버깅에 필수 — 차단 이유를 패킷 레벨로 즉시 확인 가능
- **EKS에서 Cilium 사용 시**: AWS VPC CNI 대신 Cilium을 CNI로 사용하면 ENI IP 한도 제약 없이 Pod 수 확장 가능 (IPAM 모드를 `cluster-pool`로 설정)
- **BGP 모드**: 온프레미스 환경에서 오버레이 없이 BGP 라우팅으로 Pod IP 직접 광고 → 최저 지연 달성

---

## 3. 트러블슈팅

### 3.1 주요 이슈

#### CiliumNetworkPolicy 적용 후 DNS 조회 실패

**증상**: Pod에서 외부 도메인 접근 불가, `nslookup` 타임아웃

**원인**: Egress 정책에서 DNS(53/UDP) 허용 규칙 누락

**해결 방법**:
```bash
# Hubble로 DNS 트래픽 차단 확인
hubble observe --namespace <NAMESPACE> --protocol dns --verdict DROPPED

# 정책에 DNS 예외 추가
# (위 YAML 예시의 "DNS 조회 허용" 섹션 참고)

# 임시 확인: CoreDNS Pod IP 직접 조회
kubectl exec <POD_NAME> -n <NAMESPACE> -- \
  nslookup kubernetes.default.svc.cluster.local <COREDNS_POD_IP>
```

#### kube-proxy 대체 후 NodePort 서비스 접근 불가

**증상**: `kubeProxyReplacement=true` 설정 후 NodePort로 접근 안 됨

**원인**: `k8sServiceHost`, `k8sServicePort` 설정 오류

**해결 방법**:
```bash
# Cilium 설정 확인
kubectl exec -n kube-system ds/cilium -- cilium status --verbose | grep -i "kube-proxy"

# API Server 주소 확인 후 Helm 값 수정
kubectl get endpoints kubernetes -n default
helm upgrade cilium cilium/cilium -n kube-system \
  --set k8sServiceHost=<CORRECT_API_SERVER_IP> \
  --set k8sServicePort=443
```

---

### 3.2 자주 발생하는 문제

#### Cilium Pod가 CrashLoopBackOff

**증상**: `kubectl get pods -n kube-system`에서 cilium DaemonSet Pod가 재시작 반복

**원인**: 커널 버전 미지원 (eBPF 기능 요구사항 미충족)

**해결 방법**:
```bash
# 커널 버전 확인 (최소 4.19 이상, 권장 5.10+)
uname -r

# Cilium 로그에서 구체적 오류 확인
kubectl logs -n kube-system ds/cilium --previous | tail -30

# BPF 파일시스템 마운트 확인
mount | grep bpf
# 없으면: mount bpffs /sys/fs/bpf -t bpf
```

---

## 4. 모니터링 및 확인

```bash
# Cilium 전체 상태
kubectl exec -n kube-system ds/cilium -- cilium status

# 엔드포인트 목록 (Pod별 정책 적용 상태)
kubectl exec -n kube-system ds/cilium -- cilium endpoint list

# 특정 Pod의 네트워크 정책 확인
kubectl exec -n kube-system ds/cilium -- \
  cilium endpoint get <ENDPOINT_ID>

# BPF 맵 (서비스 해시테이블) 확인
kubectl exec -n kube-system ds/cilium -- cilium bpf lb list

# WireGuard 피어 상태
kubectl exec -n kube-system ds/cilium -- cilium encrypt status

# Hubble UI 접근
kubectl port-forward -n kube-system svc/hubble-ui 12000:80
# 브라우저에서 localhost:12000 접속

# Prometheus 메트릭 (Cilium)
kubectl port-forward -n kube-system svc/cilium-agent 9090:9090
# cilium_drop_count_total, cilium_forward_count_total 등 확인
```

---

## 5. TIP

- **Tetragon 연동**: Cilium 생태계의 런타임 보안 도구 — eBPF로 시스템 콜을 추적해 컨테이너 탈출, 권한 상승 등 실시간 감지 (Falco와 유사하지만 eBPF 기반)
- **Cilium Service Mesh**: Istio 없이 Cilium만으로 L7 로드밸런싱, 헬스체크, 서킷브레이커 구현 가능 (단, Istio 대비 기능 제한적)
- **EKS 비용 절감**: AWS VPC CNI 대신 Cilium IPAM 사용 시 ENI 수를 줄일 수 있어 ENI당 요금 절감 가능
- **XDP 모드**: 지원되는 NIC 드라이버에서 XDP(eXpress Data Path) 활성화 시 커널 네트워크 스택 우회 → DDoS 방어, 로드밸런서 성능 극대화
