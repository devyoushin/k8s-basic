# Istio mTLS — 서비스 메시 내부 암호화

## 1. 개요 및 비유

Istio mTLS(Mutual TLS)는 서비스 메시 내 모든 Pod 간 통신을 자동으로 암호화하고 상호 인증하는 메커니즘입니다.
비유: 일반 TLS가 "신분증 확인 없는 보안 터널"이라면, mTLS는 "양쪽 모두 신분증을 확인하는 보안 터널"입니다. 서버도 클라이언트 신원을 검증합니다.

---

## 2. 핵심 설명

### Istio mTLS 동작 방식

```
[Service A Pod]                    [Service B Pod]
  └── Envoy Sidecar ──mTLS──────── Envoy Sidecar ──┘
       (클라이언트)    암호화 터널    (서버)
       SPIFFE ID 제시              SPIFFE ID 검증
```

- 애플리케이션 코드 변경 없이 인프라 레벨에서 자동 적용
- Istiod(Pilot)가 Envoy에 SPIFFE X.509-SVID 인증서 자동 배포
- 인증서 유효기간: 기본 24시간, 자동 갱신

### SPIFFE 워크로드 ID 형식

```
spiffe://cluster.local/ns/<namespace>/sa/<service-account>
예: spiffe://cluster.local/ns/payments/sa/payment-svc
```

- Kubernetes ServiceAccount와 1:1 매핑
- IP나 DNS 이름이 아닌 **워크로드 ID** 기반 인증 → Pod 재스케줄링에 강건

### Ambient Mesh vs Sidecar 모드 비교

| 항목 | Sidecar 모드 | Ambient Mesh 모드 |
|------|-------------|-----------------|
| 암호화 레이어 | L7 (Envoy sidecar) | L4 (ztunnel) + L7 (waypoint) |
| 리소스 오버헤드 | Pod당 Envoy 컨테이너 | 노드당 ztunnel DaemonSet |
| Pod 재시작 필요 | 사이드카 인젝션 시 필요 | 불필요 |
| 컨테이너 간 통신 | 같은 네트워크 NS → 암호화 없음 | 동일 |
| 금융권 권장 | 기존 클러스터 | 신규 클러스터 |

### WireGuard vs Istio mTLS

| 항목 | Istio mTLS | WireGuard (예: Cilium) |
|------|-----------|----------------------|
| 동작 레이어 | L7 (애플리케이션) | L3 (커널 네트워크) |
| 성능 오버헤드 | 중간 (Envoy 프록시) | 낮음 (커널 구현) |
| 세밀한 정책 | AuthorizationPolicy 지원 | IP/포트 기반만 |
| SPIFFE ID 활용 | 가능 | 불가 |
| 금융권 권장 | 정책 제어 필요 시 | 순수 성능 중심 시 |

---

## 3. YAML 적용 예시

### PeerAuthentication — STRICT mTLS 적용

```yaml
# 전역 STRICT 모드 (모든 네임스페이스)
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: default
  namespace: istio-system
spec:
  mtls:
    mode: STRICT
---
# 특정 네임스페이스 오버라이드
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: payments-mtls
  namespace: payments
spec:
  mtls:
    mode: STRICT
  portLevelMtls:
    # kubelet 헬스체크 포트는 mTLS 제외
    8080:
      mode: DISABLE
    # Istio 메트릭 포트
    15020:
      mode: DISABLE
```

### AuthorizationPolicy — SPIFFE ID 기반 서비스 간 접근 제어

```yaml
# payment-svc는 오직 api-gateway ServiceAccount에서만 접근 허용
apiVersion: security.istio.io/v1beta1
kind: AuthorizationPolicy
metadata:
  name: payment-svc-allow
  namespace: payments
spec:
  selector:
    matchLabels:
      app: payment-svc
  action: ALLOW
  rules:
    - from:
        - source:
            principals:
              - "cluster.local/ns/api-gateway/sa/api-gateway-svc"
      to:
        - operation:
            methods: ["POST", "GET"]
            paths: ["/api/v1/payments/*"]
---
# 그 외 모든 트래픽 차단 (기본 DENY)
apiVersion: security.istio.io/v1beta1
kind: AuthorizationPolicy
metadata:
  name: payment-svc-deny-all
  namespace: payments
spec:
  selector:
    matchLabels:
      app: payment-svc
  action: DENY
  rules:
    - from:
        - source:
            notPrincipals:
              - "cluster.local/ns/api-gateway/sa/api-gateway-svc"
```

### DestinationRule — TLS 모드 및 Cipher Suite 설정

```yaml
apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: payment-svc-dr
  namespace: payments
spec:
  host: payment-svc.payments.svc.cluster.local
  trafficPolicy:
    tls:
      mode: ISTIO_MUTUAL  # Istio 관리 mTLS (자동 인증서)
    connectionPool:
      http:
        h2UpgradePolicy: UPGRADE  # HTTP/2 강제 (성능 + 보안)
---
# 금융권: TLS 1.3 + 강화 Cipher Suite 강제
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  name: tls13-cipher-suite
  namespace: istio-system
spec:
  configPatches:
    - applyTo: NETWORK_FILTERED_CHAIN
      patch:
        operation: MERGE
        value:
          tls_context:
            common_tls_context:
              tls_params:
                tls_minimum_protocol_version: TLSv1_3
                cipher_suites:
                  - TLS_AES_256_GCM_SHA384
                  - TLS_CHACHA20_POLY1305_SHA256
```

### Istio Ambient Mesh — ztunnel mTLS 설정

```yaml
# Ambient Mesh 활성화 (네임스페이스 레이블)
apiVersion: v1
kind: Namespace
metadata:
  name: payments
  labels:
    istio.io/dataplane-mode: ambient
---
# L7 정책이 필요한 서비스에만 Waypoint 추가
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: payments-waypoint
  namespace: payments
  annotations:
    istio.io/service-account: payment-svc
spec:
  gatewayClassName: istio-waypoint
  listeners:
    - name: mesh
      port: 15008
      protocol: HBONE
```

### SPIRE 연동 — 외부 워크로드 ID 관리

```yaml
# SPIRE + Istio 연동 (클러스터 외부 서비스 포함)
apiVersion: install.istio.io/v1alpha1
kind: IstioOperator
spec:
  meshConfig:
    trustDomain: fintech.internal
  values:
    pilot:
      env:
        EXTERNAL_CA: "true"
        K8S_SIGNING_CERT_AND_KEY_OVERRIDE: "spire-bundle"
    global:
      caAddress: spire-server.spire:8081
```

---

## 4. 트러블슈팅

### 문제 1: mTLS STRICT 전환 후 특정 서비스 통신 불가

```bash
# 현재 네임스페이스의 mTLS 상태 확인
istioctl authn tls-check payment-svc.payments.svc.cluster.local

# 특정 Pod의 mTLS 상태 확인
istioctl proxy-config listeners <pod-name> -n payments

# PeerAuthentication 정책 충돌 확인
kubectl get peerauthentication -A

# 단계적 전환: PERMISSIVE → STRICT
# 1단계: PERMISSIVE (평문/mTLS 혼용)
# 2단계: 모든 서비스 사이드카 인젝션 확인
# 3단계: STRICT 전환
```

### 문제 2: AuthorizationPolicy 적용 후 500 에러

```bash
# Envoy 접근 로그에서 RBAC 차단 확인
kubectl logs <pod-name> -n payments -c istio-proxy | grep "RBAC"

# AuthorizationPolicy 디버그 모드
kubectl exec <pod-name> -n payments -c istio-proxy -- \
  pilot-agent request GET /config_dump | \
  python3 -m json.tool | grep -A 5 "rbac"

# 임시 우회: action을 AUDIT으로 변경 후 로그 분석
```

### 문제 3: 인증서 만료로 서비스 간 통신 중단

```bash
# Envoy가 보유한 인증서 만료일 확인
istioctl proxy-config secret <pod-name> -n payments

# Istiod에서 인증서 재발급 강제
kubectl delete secret istio.payment-svc -n payments
# Istiod가 자동으로 재발급 (수초 내)

# Istiod 인증서 갱신 로그 확인
kubectl logs -n istio-system deploy/istiod | grep "cert"
```

### 문제 4: Ambient Mesh에서 Pod 간 mTLS 동작 미확인

```bash
# ztunnel이 mTLS 처리 중인지 확인
kubectl logs -n istio-system daemonset/ztunnel | grep "HBONE\|mTLS"

# 특정 Pod의 ztunnel 연결 상태
kubectl exec -n istio-system daemonset/ztunnel -- \
  curl -s localhost:15000/config_dump | jq '.configs[] | select(.["@type"] | contains("ScopedRoutes"))'

# tcpdump로 암호화 여부 실측 확인 (평문이면 문제)
kubectl debug -it <pod-name> -n payments --image=nicolaka/netshoot -- \
  tcpdump -i eth0 -A port 8080 | grep -i "HTTP\|GET\|POST"
# 암호화된 경우 바이너리 데이터만 출력됨
```
