## 1. 개요 및 비유
**Ingress(인그레스)**는 클러스터 외부에서 내부 서비스로 들어오는 HTTP/HTTPS 트래픽을 라우팅하는 규칙 모음입니다. 하나의 외부 엔드포인트로 여러 서비스에 도메인/경로 기반 라우팅을 제공합니다.

💡 **비유하자면 '건물 1층의 안내 데스크'와 같습니다.**
건물(클러스터)에 들어온 방문객(HTTP 요청)에게 "A팀 방문이면 3층, B팀 방문이면 5층 가세요"라고 안내하는 것과 같습니다. LoadBalancer Service를 서비스마다 하나씩 만들면 비용이 많이 들지만, Ingress 하나로 모든 HTTP 트래픽을 중앙에서 관리할 수 있습니다.

## 2. 핵심 설명
* **Ingress Controller 필요:** Ingress 오브젝트 자체는 규칙만 정의합니다. 실제로 트래픽을 처리하는 것은 **Ingress Controller** (nginx-ingress, AWS ALB Controller, Traefik 등)입니다. 클러스터에 Ingress Controller를 별도로 설치해야 합니다.
* **라우팅 방식:**
  * **Host 기반:** `api.example.com` → api-service, `www.example.com` → web-service
  * **Path 기반:** `/api/*` → api-service, `/` → web-service
* **TLS 종료:** Ingress에서 TLS 인증서를 처리하여 파드들은 HTTP로만 통신하게 만들 수 있습니다.
* **IngressClass:** 클러스터에 여러 Ingress Controller가 있을 때 어느 컨트롤러가 이 Ingress를 처리할지 지정합니다.

## 3. YAML 적용 예시

### 경로 기반 라우팅
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app-ingress
  namespace: default
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /  # nginx 컨트롤러 사용 시
spec:
  ingressClassName: nginx  # 사용할 Ingress Controller 지정
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
              number: 80
      - path: /
        pathType: Prefix
        backend:
          service:
            name: web-service
            port:
              number: 80
```

### TLS 적용 (HTTPS)
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: tls-ingress
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"  # cert-manager 자동 인증서 발급
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - myapp.example.com
    secretName: myapp-tls  # TLS 인증서가 저장된 Secret
  rules:
  - host: myapp.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: web-service
            port:
              number: 80
```

### AWS ALB Ingress Controller 예시
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: alb-ingress
  annotations:
    kubernetes.io/ingress.class: alb
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/target-type: ip
    alb.ingress.kubernetes.io/listen-ports: '[{"HTTPS":443}]'
    alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:...
spec:
  rules:
  - host: api.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: api-service
            port:
              number: 80
```

## 4. 트러블 슈팅
* **Ingress 생성 후 ADDRESS가 비어있음:**
  * Ingress Controller가 설치되지 않은 것입니다. `kubectl get pods -n ingress-nginx` 등으로 컨트롤러 파드가 Running 상태인지 확인하세요.
* **404 Not Found 응답:**
  * `path`와 `pathType` 설정이 올바른지 확인하세요. `Exact`/`Prefix`/`ImplementationSpecific` 중 의도에 맞는 것을 선택해야 합니다.
  * 백엔드 Service 이름과 포트가 실제 서비스와 일치하는지 확인하세요.
* **HTTPS 인증서 오류:**
  * TLS Secret의 인증서가 만료되었거나 도메인이 불일치한 것입니다. cert-manager를 사용 중이라면 `kubectl describe certificate <이름>` 으로 발급 상태를 확인하세요.
* **특정 경로로 요청 시 백엔드에 잘못된 경로가 전달됨:**
  * nginx 컨트롤러의 경우 `nginx.ingress.kubernetes.io/rewrite-target` 어노테이션으로 경로를 재작성할 수 있습니다.
