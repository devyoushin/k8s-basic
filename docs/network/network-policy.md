## 1. 개요 및 비유
**NetworkPolicy(네트워크 폴리시)**는 파드 간 트래픽을 L3/L4 수준에서 제어하는 쿠버네티스의 방화벽 규칙입니다. 기본적으로 쿠버네티스는 모든 파드 간 통신을 허용하므로, NetworkPolicy로 명시적으로 제어해야 합니다.

💡 **비유하자면 '사무실 출입 카드 시스템'과 같습니다.**
기본 상태는 건물 내 모든 문이 열려있는 것과 같습니다. NetworkPolicy는 "개발팀 카드(라벨)를 가진 사람만 서버실에 들어올 수 있다"는 규칙을 만드는 것입니다. 규칙을 명시하지 않으면 모두 통과입니다.

## 2. 핵심 설명
* **기본값은 전체 허용:** NetworkPolicy가 없는 파드는 모든 트래픽을 허용합니다. 보안을 위해 `default-deny-all` 정책부터 적용하고 필요한 것만 허용(화이트리스트)하는 방식을 권장합니다.
* **CNI 지원 필요:** NetworkPolicy는 CNI 플러그인이 실제로 구현합니다. Calico, Cilium, Weave는 지원하지만 **Flannel은 기본적으로 지원하지 않습니다.**
* **Ingress / Egress:**
  * `ingress`: 파드로 들어오는 트래픽 제어
  * `egress`: 파드에서 나가는 트래픽 제어
* **선택자(Selector) 3가지:**
  * `podSelector`: 특정 라벨의 파드에서/로 허용
  * `namespaceSelector`: 특정 네임스페이스에서/로 허용
  * `ipBlock`: 특정 CIDR 대역에서/로 허용

## 3. YAML 적용 예시

### 1단계: 네임스페이스 전체 트래픽 차단 (Default Deny)
```yaml
# 모든 인그레스 차단
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-ingress
  namespace: production
spec:
  podSelector: {}       # 네임스페이스 내 모든 파드에 적용
  policyTypes:
  - Ingress             # 인그레스 트래픽만 차단 (egress는 허용 유지)
```

### 2단계: 필요한 트래픽만 허용 (Whitelisting)
```yaml
# frontend 파드 → backend 파드의 8080 포트만 허용
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-frontend-to-backend
  namespace: production
spec:
  podSelector:
    matchLabels:
      app: backend      # 이 정책이 적용될 파드 (트래픽을 받는 쪽)
  policyTypes:
  - Ingress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: frontend # frontend 라벨 파드에서 오는 트래픽만 허용
    ports:
    - protocol: TCP
      port: 8080
```

### 크로스 네임스페이스 허용
```yaml
# monitoring 네임스페이스의 prometheus가 모든 네임스페이스 파드의 메트릭 수집 허용
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-prometheus-scrape
  namespace: production
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: monitoring
      podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 9090
```

### Egress 제어 (외부 DB 접근 허용 + 나머지 차단)
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: backend-egress
  namespace: production
spec:
  podSelector:
    matchLabels:
      app: backend
  policyTypes:
  - Egress
  egress:
  - to:
    - podSelector:
        matchLabels:
          app: mysql
    ports:
    - protocol: TCP
      port: 3306
  - to:                  # CoreDNS 접근 허용 (없으면 DNS 조회 불가)
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: kube-system
    ports:
    - protocol: UDP
      port: 53
    - protocol: TCP
      port: 53
```

> **주의:** Egress를 차단할 경우 반드시 **DNS(포트 53) 트래픽은 허용**해야 합니다. 빠뜨리면 서비스 이름으로 통신이 전혀 안 됩니다.

## 4. 트러블 슈팅
* **NetworkPolicy 적용 후 통신이 전부 안 됨:**
  * Egress 정책에서 DNS(포트 53) 허용을 빠뜨렸을 가능성이 높습니다. `kubectl exec`로 파드에 접속하여 `nslookup kubernetes.default` 가 되는지 먼저 확인하세요.
* **정책을 만들었는데 효과가 없음:**
  * CNI 플러그인이 NetworkPolicy를 지원하지 않는 것입니다. `kubectl describe pod -n kube-system -l k8s-app=calico-node` 등으로 CNI를 확인하세요.
* **정책 디버깅이 어려울 때:**
  * Cilium을 사용 중이라면 `cilium policy trace` 명령어로 특정 트래픽이 허용/차단되는 이유를 추적할 수 있습니다.
  * Calico는 `calicoctl` 도구로 정책 효과를 시뮬레이션할 수 있습니다.
* **and 조건 vs or 조건 혼동:**
  * 같은 `from` 항목 안에 `podSelector`와 `namespaceSelector`를 함께 쓰면 **AND** 조건입니다.
  * 별도 `from` 항목으로 나누면 **OR** 조건입니다.
  ```yaml
  # AND: monitoring 네임스페이스의 prometheus 파드만 허용
  from:
  - namespaceSelector:
      matchLabels:
        name: monitoring
    podSelector:
      matchLabels:
        app: prometheus

  # OR: monitoring 네임스페이스 전체 OR prometheus 라벨 파드
  from:
  - namespaceSelector:
      matchLabels:
        name: monitoring
  - podSelector:
      matchLabels:
        app: prometheus
  ```
