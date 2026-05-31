## 1. 개요 및 비유
**Service Mesh(서비스 메시)**는 마이크로서비스 간의 모든 네트워크 통신을 애플리케이션 코드 밖에서 투명하게 제어하는 인프라 레이어입니다. Istio가 가장 널리 쓰이며, 각 파드 옆에 사이드카 프록시(Envoy)를 붙여 트래픽을 가로채는 방식으로 동작합니다.

💡 **비유하자면 '모든 우편물에 자동으로 붙는 스마트 추적 스티커'와 같습니다.**
편지(요청)를 보낼 때 우체국 직원(개발자)이 추적 번호를 직접 적을 필요 없이, 우편함(사이드카 프록시)이 자동으로 추적 스티커(메트릭, 트레이싱, mTLS)를 붙여 배달합니다. 어느 편지가 어디서 어디로 가는지, 암호화는 됐는지 모두 자동으로 관리됩니다.

## 2. 핵심 설명

### Service Mesh가 해결하는 문제들

| 기능 | 설명 |
|---|---|
| **mTLS (상호 TLS)** | 서비스 간 통신을 자동으로 암호화 + 인증. 코드 변경 없음 |
| **트래픽 관리** | 카나리 배포, A/B 테스트, 회로 차단기(Circuit Breaker), 재시도 |
| **관찰 가능성** | 서비스 간 레이턴시, 에러율, 트래픽 경로 시각화 |
| **접근 제어** | L7(HTTP 메서드, 경로, 헤더) 수준의 정책 적용 |

### Istio 핵심 구성 요소

```
┌─────────────────────────────────────┐
│           Control Plane             │
│  istiod (Pilot + Citadel + Galley)  │
│  - 설정 배포 / 인증서 관리 / 서비스 디스커버리│
└────────────────┬────────────────────┘
                 │ xDS 프로토콜로 설정 전달
    ┌────────────▼────────────┐
    │       Data Plane        │
    │  [앱 컨테이너 | Envoy]  │  ← 파드마다 사이드카 자동 주입
    │  [앱 컨테이너 | Envoy]  │
    └─────────────────────────┘
```

### 주요 Istio 리소스

| 리소스 | 역할 |
|---|---|
| `VirtualService` | 트래픽 라우팅 규칙 정의 (카나리, 헤더 기반 라우팅 등) |
| `DestinationRule` | 목적지 정책 정의 (로드밸런싱, 서킷 브레이커, mTLS) |
| `Gateway` | 클러스터 외부에서 들어오는 트래픽 진입점 |
| `PeerAuthentication` | mTLS 모드 설정 (STRICT / PERMISSIVE) |
| `AuthorizationPolicy` | L7 수준 접근 제어 |

## 3. YAML 적용 예시

### 사이드카 자동 주입 활성화
```bash
# 네임스페이스에 라벨을 달면 이후 생성되는 파드에 Envoy가 자동 주입됨
kubectl label namespace production istio-injection=enabled
```

### VirtualService - 카나리 배포 (v1: 90%, v2: 10%)
```yaml
apiVersion: networking.istio.io/v1
kind: VirtualService
metadata:
  name: reviews
  namespace: production
spec:
  hosts:
  - reviews          # 이 서비스로 향하는 트래픽을 제어
  http:
  - route:
    - destination:
        host: reviews
        subset: v1   # DestinationRule에서 정의한 subset
      weight: 90
    - destination:
        host: reviews
        subset: v2
      weight: 10
```

### DestinationRule - 서브셋 정의 + 서킷 브레이커
```yaml
apiVersion: networking.istio.io/v1
kind: DestinationRule
metadata:
  name: reviews
  namespace: production
spec:
  host: reviews
  subsets:
  - name: v1
    labels:
      version: v1    # 파드의 라벨로 버전 구분
  - name: v2
    labels:
      version: v2
  trafficPolicy:
    connectionPool:
      tcp:
        maxConnections: 100
    outlierDetection:        # 서킷 브레이커 설정
      consecutive5xxErrors: 5       # 5xx 에러가 5번 연속 발생하면
      interval: 30s                 # 30초 간격으로 체크
      baseEjectionTime: 30s         # 30초 동안 해당 인스턴스 제외
```

### PeerAuthentication - mTLS 강제 적용
```yaml
# production 네임스페이스 내 모든 통신을 mTLS로 강제
apiVersion: security.istio.io/v1
kind: PeerAuthentication
metadata:
  name: default
  namespace: production
spec:
  mtls:
    mode: STRICT   # PERMISSIVE: mTLS 선택적, STRICT: 반드시 mTLS
```

### AuthorizationPolicy - L7 접근 제어
```yaml
# frontend 서비스만 backend의 GET /api/* 호출 허용
apiVersion: security.istio.io/v1
kind: AuthorizationPolicy
metadata:
  name: backend-authz
  namespace: production
spec:
  selector:
    matchLabels:
      app: backend
  action: ALLOW
  rules:
  - from:
    - source:
        principals: ["cluster.local/ns/production/sa/frontend-sa"]  # ServiceAccount 기반
    to:
    - operation:
        methods: ["GET"]
        paths: ["/api/*"]
```

### Gateway - 외부 트래픽 진입점
```yaml
apiVersion: networking.istio.io/v1
kind: Gateway
metadata:
  name: main-gateway
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
      mode: SIMPLE
      credentialName: myapp-tls-secret
    hosts:
    - "api.example.com"
```

## 4. 트러블 슈팅

* **사이드카가 주입되지 않음:**
  * 네임스페이스에 `istio-injection=enabled` 라벨이 있는지 확인하세요.
  * 라벨은 이미 떠 있는 파드에는 소급 적용되지 않습니다. `kubectl rollout restart deployment <이름>` 으로 파드를 재시작해야 합니다.

* **mTLS STRICT 모드 적용 후 통신이 안 됨:**
  * 사이드카가 없는 파드(레거시 앱, 특정 시스템 파드)에서 오는 트래픽이 차단된 것입니다.
  * 먼저 `PERMISSIVE` 모드로 전환하여 영향 범위를 파악한 뒤, 모든 파드에 사이드카가 주입된 것을 확인하고 `STRICT`로 전환하세요.

* **VirtualService가 적용되지 않음:**
  * `VirtualService`의 `hosts`가 실제 쿠버네티스 Service 이름과 정확히 일치해야 합니다.
  * `kubectl get virtualservice -n production -o yaml` 로 설정을 확인하고, `istioctl analyze` 명령어로 설정 오류를 자동 진단하세요.

* **Envoy 사이드카로 인한 레이턴시 증가:**
  * 사이드카 프록시를 거치면 약 1~2ms의 오버헤드가 발생합니다. 고성능이 필요한 서비스는 `sidecar.istio.io/inject: "false"` 어노테이션으로 사이드카 주입을 선택적으로 제외할 수 있습니다.
  * 또는 Cilium의 eBPF 기반 서비스 메시(`Cilium Service Mesh`)를 검토하세요. 사이드카 없이 커널 레벨에서 동작하여 오버헤드가 훨씬 적습니다.

* **디버깅 도구:**
  ```bash
  # 설정 오류 자동 분석
  istioctl analyze -n production

  # 특정 파드의 Envoy 설정 확인
  istioctl proxy-config cluster <파드명> -n production

  # 트래픽 흐름 추적
  istioctl proxy-config log <파드명> --level debug
  ```
