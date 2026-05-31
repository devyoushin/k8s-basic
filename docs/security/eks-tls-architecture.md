# EKS 종단간 암호화 아키텍처 (금융권 수준)

## 1. 개요 및 비유

금융권 EKS 환경에서 요구되는 모든 통신 구간을 TLS로 보호하는 다층 암호화 아키텍처입니다.
비유: 은행 금고 건물처럼 외벽(LB), 복도(mesh), 금고실(pod) 각각의 잠금장치가 독립적으로 작동합니다.

---

## 2. 핵심 설명

### 전체 암호화 레이어 구조

```
[클라이언트 앱/브라우저]
        │ TLS 1.3 (SSL Pinning 적용)
        ▼
[AWS ALB / NLB]  ← 1차 TLS Termination (ALB) 또는 Passthrough (NLB)
        │ TLS (Istio Ingress Gateway로 재암호화)
        ▼
[Istio Ingress Gateway Pod]
        │ Istio mTLS (SPIFFE 워크로드 ID 기반)
        ▼
[API Gateway Pod]
        │ Istio mTLS (ambient mesh 또는 sidecar)
        ▼
[Backend Service Pod]
        │ TLS (OriginationPolicy로 강제)
        ▼
[External: Redis / RDS / 외부 API]
```

---

### 구간별 암호화 전략

#### 구간 1: 클라이언트 → ALB (1차 Termination)

| 항목 | 설정값 |
|------|--------|
| 프로토콜 | TLS 1.3 (TLS 1.2 최소) |
| Cipher Suite | `ELBSecurityPolicy-TLS13-1-3-2021-06` |
| 인증서 | ACM Public Certificate (도메인 검증) |
| mTLS | AWS ALB mTLS 지원 활용 (클라이언트 인증서 검증) |
| SSL Pinning | 클라이언트 앱에서 SPKI hash 핀 |

#### 구간 2: ALB → Istio Ingress Gateway (재암호화)

- ALB에서 Termination 후 Istio Ingress Gateway로 HTTP/2 또는 재암호화된 TLS 전달
- NLB Passthrough 모드: TLS 암호화 트래픽을 직접 Ingress Gateway Pod으로 전달 (ALB 불필요)

#### 구간 3: Istio Ingress Gateway → Pod (Istio mTLS)

- Istio ambient mesh 또는 sidecar 프록시가 자동으로 mTLS 적용
- SPIFFE X.509-SVID 인증서 기반 워크로드 ID 사용
- `PeerAuthentication`: STRICT 모드 강제

#### 구간 4: Pod ↔ Pod (서비스 메시 내부)

- Istio mTLS STRICT 또는 WireGuard 커널 레벨 암호화
- 네임스페이스별 `PeerAuthentication` 정책으로 평문 통신 차단

#### 구간 5: Pod 내부 컨테이너 간 통신

- **암호화 불필요**: 동일 네트워크 네임스페이스 공유 (같은 loopback interface)
- Istio ambient mesh 사용 시도 동일 (ztunnel이 pod 경계에서만 동작)

#### 구간 6: Pod → External Service (Redis, RDS 등)

- Redis: `tls-auth` 강제, Stunnel 또는 Redis TLS 모드
- RDS: `sslmode=verify-full`, 루트 CA 인증서 포함
- 외부 API: Istio `ServiceEntry` + `DestinationRule`로 origination TLS 강제
- Egress Gateway를 통한 아웃바운드 통제 및 감사 로그

---

### ALB vs NLB 선택 기준

| 시나리오 | 권장 LB | 이유 |
|----------|---------|------|
| 일반 HTTPS API | ALB | L7 라우팅, mTLS 지원, WAF 연동 |
| gRPC / HTTP/2 | ALB (gRPC 지원) | 프로토콜 수준 라우팅 |
| 저지연 고성능 | NLB Passthrough | L4, TLS Termination 없이 직접 전달 |
| 클라이언트 인증서 검증 | ALB mTLS | 브라우저/앱 클라이언트 인증서 검증 |

NLB Passthrough 사용 시 주의사항:
- TLS Termination을 Istio Ingress Gateway에서 직접 수행
- AWS Private CA(PCA)에서 발급한 인증서를 Ingress Gateway에 직접 마운트
- `kubectl` 시크릿 또는 cert-manager로 인증서 관리 필수

---

## 3. YAML 적용 예시

### ALB Ingress — TLS 1.3 + mTLS 활성화

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: api-ingress
  namespace: payments
  annotations:
    kubernetes.io/ingress.class: alb
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:ap-northeast-2:123456789:certificate/xxxx
    # TLS 1.3 전용 정책
    alb.ingress.kubernetes.io/ssl-policy: ELBSecurityPolicy-TLS13-1-3-2021-06
    alb.ingress.kubernetes.io/listen-ports: '[{"HTTPS": 443}]'
    # mTLS: 클라이언트 인증서 검증 (AWS ALB mTLS)
    alb.ingress.kubernetes.io/mutual-authentication: >
      [{"port": 443, "mode": "verify", "trustStore": "arn:aws:elasticloadbalancing:..."}]
spec:
  rules:
    - host: api.fintech.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: istio-ingressgateway
                port:
                  number: 443
```

### Istio PeerAuthentication — STRICT mTLS 전역 적용

```yaml
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: default
  namespace: istio-system  # 전역 정책
spec:
  mtls:
    mode: STRICT  # 평문 트래픽 전면 차단
---
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: payments-strict
  namespace: payments
spec:
  mtls:
    mode: STRICT
```

### Istio DestinationRule — External Redis TLS Origination

```yaml
apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: redis-tls
  namespace: payments
spec:
  host: redis.internal.company.com
  trafficPolicy:
    tls:
      mode: SIMPLE
      caCertificates: /etc/ssl/certs/company-root-ca.pem
      sni: redis.internal.company.com
---
apiVersion: networking.istio.io/v1beta1
kind: ServiceEntry
metadata:
  name: redis-external
  namespace: payments
spec:
  hosts:
    - redis.internal.company.com
  ports:
    - number: 6380
      name: tls-redis
      protocol: TLS
  resolution: DNS
  location: MESH_EXTERNAL
```

### NLB Passthrough — Istio Ingress Gateway 직접 TLS

```yaml
# NLB Passthrough 시: Istio Gateway에서 TLS 종단
apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: api-gateway
  namespace: istio-system
spec:
  selector:
    istio: ingressgateway
  servers:
    - port:
        number: 443
        name: https
        protocol: HTTPS
      tls:
        mode: MUTUAL  # 클라이언트 인증서 검증
        credentialName: api-gateway-cert  # cert-manager로 관리
        minProtocolVersion: TLSV1_3
        cipherSuites:
          - TLS_AES_256_GCM_SHA384
      hosts:
        - "api.fintech.com"
```

---

## 4. 트러블슈팅

### 문제 1: ALB mTLS 활성화 후 기존 클라이언트 통신 불가

```
원인: Trust Store에 클라이언트 CA 인증서 미등록
확인: ALB 액세스 로그에서 "CLIENTCERT_" 접두사 에러 확인
해결:
  1. 클라이언트 CA 인증서를 AWS Certificate Manager Trust Store에 등록
  2. Trust Store ARN을 mutual-authentication 어노테이션에 설정
  3. 단계적 롤아웃: mode를 "passthrough" → "verify"로 순차 변경
```

### 문제 2: Istio STRICT mTLS 전환 후 헬스체크 실패

```
원인: kubelet 헬스체크는 mTLS를 통과하지 못함
해결: 특정 포트만 예외 처리
```

```yaml
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: payments-strict
  namespace: payments
spec:
  mtls:
    mode: STRICT
  portLevelMtls:
    15020:  # Istio 헬스체크 포트
      mode: DISABLE
    8080:   # 앱 헬스체크 포트 (kubelet 접근)
      mode: DISABLE
```

### 문제 3: NLB Passthrough 모드에서 클라이언트 IP 손실

```
원인: L4 Passthrough 시 X-Forwarded-For 헤더 없음
해결: Proxy Protocol v2 활성화
```

```yaml
# NLB 서비스 어노테이션
service.beta.kubernetes.io/aws-load-balancer-proxy-protocol: "*"
```

```yaml
# Istio Ingress Gateway EnvoyFilter로 Proxy Protocol 활성화
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  name: proxy-protocol
  namespace: istio-system
spec:
  workloadSelector:
    labels:
      istio: ingressgateway
  configPatches:
    - applyTo: LISTENER
      patch:
        operation: MERGE
        value:
          listener_filters:
            - name: envoy.filters.listener.proxy_protocol
```

### 문제 4: External Redis TLS 연결 실패

```bash
# Pod 내에서 Redis TLS 연결 테스트
kubectl exec -n payments deploy/payment-svc -- \
  redis-cli -h redis.internal.company.com -p 6380 \
  --tls --cacert /etc/ssl/certs/company-root-ca.pem ping

# Istio 사이드카 로그에서 TLS handshake 확인
kubectl logs -n payments deploy/payment-svc -c istio-proxy | grep -i "tls\|ssl"
```
