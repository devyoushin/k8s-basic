## 1. 개요 및 비유

Gateway API는 Ingress를 대체하는 차세대 Kubernetes 네트워킹 API입니다. Ingress가 단일 오브젝트로 모든 것을 처리했다면, Gateway API는 **역할별로 책임을 분리**합니다.

💡 **비유하자면 도로 인프라 관리와 같습니다.**
인프라팀(GatewayClass)이 도로 종류를 결정하고, 네트워크팀(Gateway)이 실제 도로를 만들고, 앱팀(HTTPRoute)이 표지판(라우팅 규칙)을 설치합니다. 서로 독립적으로 관리합니다.

---

## 2. Ingress vs Gateway API 비교

```
Ingress (구):
┌────────────────────────────────────┐
│  Ingress (단일 오브젝트)            │
│  - 클래스 지정 (annotations)        │
│  - 호스트/경로 라우팅               │
│  - TLS 설정                         │
│  - 모든 기능이 annotations로 설정   │
│    → 벤더마다 annotations 제각각   │
└────────────────────────────────────┘

Gateway API (신):
┌─────────────┐  ┌─────────────┐  ┌─────────────────┐
│GatewayClass │  │  Gateway    │  │   HTTPRoute      │
│(인프라팀)   │  │(네트워크팀) │  │  (애플리케이션팀)│
│             │  │             │  │                  │
│어떤 컨트롤러│  │실제 LB/프록 │  │경로 라우팅 규칙  │
│를 사용할지  │  │시 인스턴스  │  │헤더/가중치/필터  │
└─────────────┘  └─────────────┘  └─────────────────┘

장점:
- 표준화된 API (벤더 중립)
- 역할 분리 (멀티테넌시 친화)
- 고급 트래픽 기능 기본 포함
- TCPRoute, GRPCRoute, TLSRoute 등 다양한 프로토콜
```

---

## 3. 핵심 리소스 상세

### 3.1 GatewayClass — 컨트롤러 정의

```yaml
# 클러스터 관리자가 설정
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: nginx
spec:
  controllerName: k8s.nginx.org/nginx-gateway-controller
  # 또는:
  # k8s-gateway.nginx.org/nginx-gateway (NGINX)
  # gateway.envoyproxy.io/gatewayclass-controller (Envoy)
  # cilium.io/cilium-gateway-controller (Cilium)
  description: "NGINX 기반 Gateway"
```

### 3.2 Gateway — 실제 로드밸런서

```yaml
# 네트워크팀이 설정
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: prod-gateway
  namespace: gateway-infra
spec:
  gatewayClassName: nginx

  listeners:
  - name: http
    protocol: HTTP
    port: 80
    allowedRoutes:
      namespaces:
        from: Selector              # 특정 네임스페이스만 허용
        selector:
          matchLabels:
            gateway-access: "true"

  - name: https
    protocol: HTTPS
    port: 443
    tls:
      mode: Terminate
      certificateRefs:
      - kind: Secret
        name: prod-tls-secret
        namespace: gateway-infra
    allowedRoutes:
      namespaces:
        from: All                   # 모든 네임스페이스 허용
```

### 3.3 HTTPRoute — 라우팅 규칙

```yaml
# 앱팀이 자신의 네임스페이스에서 설정
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-app-route
  namespace: production   # 앱팀 네임스페이스
spec:
  parentRefs:
  - name: prod-gateway
    namespace: gateway-infra
    sectionName: https    # Gateway의 https 리스너에 연결

  hostnames:
  - "myapp.example.com"

  rules:
  # 기본 라우팅
  - matches:
    - path:
        type: PathPrefix
        value: /api
    backendRefs:
    - name: api-service
      port: 8080
      weight: 100

  # 헤더 기반 라우팅
  - matches:
    - headers:
      - name: x-version
        value: v2
    backendRefs:
    - name: api-service-v2
      port: 8080

  # 카나리 배포 (가중치 기반)
  - matches:
    - path:
        type: PathPrefix
        value: /
    backendRefs:
    - name: app-stable
      port: 80
      weight: 90          # 90% 트래픽
    - name: app-canary
      port: 80
      weight: 10          # 10% 트래픽 (카나리)
```

---

## 4. 고급 트래픽 기능

### 4.1 HTTPRoute 필터 — 요청/응답 변환

```yaml
rules:
- matches:
  - path:
      type: PathPrefix
      value: /old-api
  filters:
  # URL 리다이렉트
  - type: RequestRedirect
    requestRedirect:
      path:
        type: ReplacePrefixMatch
        replacePrefixMatch: /new-api
      statusCode: 301

  # 헤더 추가
  - type: RequestHeaderModifier
    requestHeaderModifier:
      add:
      - name: x-forwarded-host
        value: myapp.example.com
      remove:
      - x-internal-token     # 보안상 내부 헤더 제거

  # 응답 헤더 수정
  - type: ResponseHeaderModifier
    responseHeaderModifier:
      add:
      - name: x-cache-control
        value: "no-store"
```

### 4.2 ReferenceGrant — 크로스 네임스페이스 접근 제어

Gateway API는 크로스 네임스페이스 참조 시 명시적 허가가 필요합니다.

```yaml
# gateway-infra 네임스페이스에서 production 네임스페이스의 Secret 사용 허가
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: allow-gateway-to-use-tls-secret
  namespace: production        # 허가하는 리소스가 있는 네임스페이스
spec:
  from:
  - group: gateway.networking.k8s.io
    kind: Gateway
    namespace: gateway-infra   # 어디서 참조 허가를 주는지
  to:
  - group: ""
    kind: Secret
    name: prod-tls-secret      # 특정 Secret만 허가
```

### 4.3 GRPCRoute — gRPC 서비스 라우팅

```yaml
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: GRPCRoute
metadata:
  name: grpc-route
spec:
  parentRefs:
  - name: prod-gateway
  hostnames:
  - grpc.example.com
  rules:
  - matches:
    - method:
        service: mypackage.UserService  # gRPC 서비스명
        method: GetUser                 # 메서드명 (생략 시 모든 메서드)
    backendRefs:
    - name: user-service
      port: 9090
```

---

## 5. Ingress에서 Gateway API 마이그레이션

```
마이그레이션 전략:
1단계: Gateway API 설치 (CRD 추가)
2단계: GatewayClass/Gateway 생성
3단계: Ingress → HTTPRoute 변환 (동시 운영 가능)
4단계: DNS 또는 가중치를 통해 점진적 전환
5단계: Ingress 제거
```

```bash
# Gateway API CRD 설치
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.1.0/standard-install.yaml

# 실험적 기능(GRPCRoute, TCPRoute 등) 포함
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.1.0/experimental-install.yaml

# 설치 확인
kubectl get crd | grep gateway
# gateways.gateway.networking.k8s.io
# httproutes.gateway.networking.k8s.io
# grpcroutes.gateway.networking.k8s.io
```

```yaml
# Ingress (기존)
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-ingress
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  rules:
  - host: myapp.example.com
    http:
      paths:
      - path: /api
        pathType: Prefix
        backend:
          service:
            name: api-service
            port:
              number: 8080

# HTTPRoute (대응하는 Gateway API)
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-route
spec:
  parentRefs:
  - name: prod-gateway
  hostnames: ["myapp.example.com"]
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /api
    filters:
    - type: URLRewrite
      urlRewrite:
        path:
          type: ReplacePrefixMatch
          replacePrefixMatch: /    # annotations 대신 표준 필드
    backendRefs:
    - name: api-service
      port: 8080
```

---

## 6. 트러블슈팅

* **HTTPRoute가 연결 안 됨 (Accepted: False):**
  ```bash
  # Route 상태 확인
  kubectl describe httproute my-app-route
  # status.parents[0].conditions 확인
  # Accepted: False → parentRef 설정 문제
  # ResolvedRefs: False → backendRef 서비스 없음

  # Gateway가 Route를 허용하는지 확인
  kubectl describe gateway prod-gateway
  # listener의 allowedRoutes 설정 확인
  ```

* **크로스 네임스페이스 TLS Secret 참조 실패:**
  ```bash
  # ReferenceGrant 존재 여부 확인
  kubectl get referencegrant -n production

  # Gateway 상태에서 에러 확인
  kubectl describe gateway prod-gateway | grep -A5 "Conditions"
  # InvalidCertificateRef → ReferenceGrant 없거나 Secret 없음
  ```

* **카나리 가중치가 적용 안 됨:**
  ```bash
  # 컨트롤러가 가중치를 지원하는지 확인
  kubectl describe gatewayclass nginx | grep "Supported Features"

  # 실제 트래픽 분산 확인
  for i in {1..20}; do
    curl -s myapp.example.com | grep "version"
  done
  ```
