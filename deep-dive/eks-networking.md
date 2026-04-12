## 1. 개요 및 비유

**EKS 네트워킹(EKS Networking)**은 AWS VPC 위에서 동작하는 EKS 클러스터의 파드 네트워킹, 로드밸런서 연동, 보안그룹 설정을 포괄합니다.

💡 **비유하자면 아파트 단지(VPC)에 입주한 회사(EKS 클러스터)와 같습니다.**
각 직원(파드)이 실제 동 호수(VPC IP)를 부여받아 외부에서 직접 연락할 수 있고(ENI), 안내 데스크(NLB/ALB)가 방문자(외부 트래픽)를 적절한 직원에게 연결해줍니다.

---

## 2. 핵심 설명

### VPC CNI (aws-node)
EKS는 기본적으로 **Amazon VPC CNI 플러그인**을 사용합니다.

- **ENI(Elastic Network Interface) 기반:** 파드가 VPC 서브넷의 실제 IP를 직접 부여받음
- **파드 IP = VPC IP:** 파드 간 통신에 NAT 없음, VPC 라우팅 테이블로 직접 통신
- **노드당 파드 수 제한:** 노드 EC2 타입에 따라 ENI 수와 IP 수가 제한됨
  - `최대 파드 수 = (ENI 수 × (ENI당 IP 수 - 1)) + 2`

```bash
# 노드 타입별 ENI 제한 확인
aws ec2 describe-instance-types \
  --instance-types m5.large \
  --query 'InstanceTypes[].NetworkInfo'

# m5.large: ENI 3개, ENI당 IP 10개 → 최대 파드 = (3 × 9) + 2 = 29개
```

### NLB vs ALB 선택 기준

| 항목 | NLB (Network Load Balancer) | ALB (Application Load Balancer) |
|---|---|---|
| OSI 계층 | L4 (TCP/UDP) | L7 (HTTP/HTTPS) |
| K8s 오브젝트 | Service (type: LoadBalancer) | Ingress |
| 컨트롤러 | AWS Load Balancer Controller | AWS Load Balancer Controller |
| 헬스체크 | TCP 포트 또는 HTTP | HTTP/HTTPS 경로 |
| 고정 IP | 가능 (Elastic IP) | 불가 (DNS만 지원) |
| WebSocket/gRPC | 기본 지원 | 지원 (단, gRPC는 추가 설정) |
| 권장 사용 | TCP 서비스, gRPC, 고정 IP 필요 | HTTP API, 경로 기반 라우팅 |

### 타깃 그룹 모드: Instance vs IP

```
Instance 모드:           IP 모드 (권장):
[NLB]                   [NLB]
  ↓ NodePort              ↓ 직접 파드 IP로 전달
[Node]                  [Pod]
  ↓ iptables DNAT
[Pod]

IP 모드 장점:
- 이중 홉(double hop) 제거 → 지연 감소
- 파드 직접 헬스체크 → 정확한 헬스체크
- 파드 종료 시 즉시 타깃에서 제거
```

---

## 3. YAML 적용 예시

### NLB 연동 (IP 타깃 모드, AWS Load Balancer Controller)
```yaml
apiVersion: v1
kind: Service
metadata:
  name: web-app
  annotations:
    # AWS Load Balancer Controller 사용 (인스턴스 모드 대신 IP 모드)
    service.beta.kubernetes.io/aws-load-balancer-type: "external"
    service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: "ip"  # 파드 IP 직접 타깃

    # 헬스체크 설정
    service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol: "HTTP"
    service.beta.kubernetes.io/aws-load-balancer-healthcheck-path: "/healthz"
    service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval: "10"   # 초
    service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout: "5"
    service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold: "2"
    service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold: "2"

    # Connection Draining (파드 종료 시 기존 연결 유지 시간)
    service.beta.kubernetes.io/aws-load-balancer-target-group-attributes: |
      deregistration_delay.timeout_seconds=30

    # 내부 NLB (인터넷 노출 안 함)
    service.beta.kubernetes.io/aws-load-balancer-scheme: "internal"
spec:
  type: LoadBalancer
  selector:
    app: web-app
  ports:
  - port: 80
    targetPort: 8080
    protocol: TCP
```

### ALB Ingress (경로 기반 라우팅)
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web-app
  annotations:
    kubernetes.io/ingress.class: "alb"
    alb.ingress.kubernetes.io/scheme: "internet-facing"
    alb.ingress.kubernetes.io/target-type: "ip"          # IP 타깃 모드
    alb.ingress.kubernetes.io/healthcheck-path: "/healthz"
    alb.ingress.kubernetes.io/healthcheck-interval-seconds: "15"
    alb.ingress.kubernetes.io/success-codes: "200"

    # Connection Draining
    alb.ingress.kubernetes.io/target-group-attributes: |
      deregistration_delay.timeout_seconds=30

    # 인증서 자동 연결
    alb.ingress.kubernetes.io/certificate-arn: "arn:aws:acm:..."
    alb.ingress.kubernetes.io/ssl-redirect: "443"
spec:
  rules:
  - host: api.example.com
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

### 보안그룹 파드 직접 연결 (Security Group for Pods)
```yaml
# 파드 레벨 보안그룹 설정 (ENI 기반)
apiVersion: vpcresources.k8s.aws/v1beta1
kind: SecurityGroupPolicy
metadata:
  name: web-app-sgp
  namespace: default
spec:
  podSelector:
    matchLabels:
      app: web-app
  securityGroups:
    groupIds:
    - sg-0123456789abcdef0   # 파드에 직접 적용할 보안그룹 ID
```

---

## 4. 트러블 슈팅

### 사례 1: Rolling Update 중 NLB Unhealthy 발생

**원인 분석:**
```
배포 전: resource request 250Mi → 1000Mi 증가

[1] 기존 파드(250Mi) Terminating
    → K8s Endpoint에서 파드 IP 제거
    → NLB 타깃 그룹에서 draining 시작 (deregistration_delay 동안 기존 연결 유지)

[2] 새 파드(1000Mi) 생성 시도
    → 노드 메모리 여유 없음 → Pending 상태

[3] Running 파드 수 감소 (healthy 타깃 감소)
    → unhealthy threshold 미달 → NLB Unhealthy 판정
```

**해결:**
```yaml
# 1. maxUnavailable: 0 으로 새 파드 먼저 올리기
strategy:
  rollingUpdate:
    maxUnavailable: 0
    maxSurge: 1

# 2. preStop sleep으로 NLB 드레이닝 시간 확보
lifecycle:
  preStop:
    exec:
      command: ["/bin/sh", "-c", "sleep 30"]  # deregistration_delay와 맞추기

# 3. 배포 전 노드 여유 확인
# kubectl describe nodes | grep -A 5 "Allocated resources"
```

---

### 사례 2: 파드 수가 줄어들지 않음 (IP 소진)

**원인:** 노드 타입의 ENI/IP 한도 초과로 새 파드 Pending

```bash
# 노드별 파드 수 및 IP 사용량 확인
kubectl describe node <node-name> | grep -A 5 "Allocated resources"
kubectl get pods -A --field-selector spec.nodeName=<node-name> | wc -l

# VPC CNI 로그 확인
kubectl logs -n kube-system -l k8s-app=aws-node --tail=50

# 해결: 노드 타입 업그레이드 또는 ENABLE_PREFIX_DELEGATION 활성화
# (Prefix Delegation: /28 블록 단위로 IP 할당 → 노드당 파드 수 대폭 증가)
```

```bash
# Prefix Delegation 활성화 (EKS 1.28+)
kubectl set env daemonset aws-node \
  -n kube-system \
  ENABLE_PREFIX_DELEGATION=true \
  WARM_PREFIX_TARGET=1
```

---

### 사례 3: NLB → 파드 직접 통신 시 timeout

**원인:** 보안그룹에서 NLB 소스 IP 허용 안 됨 (IP 타깃 모드 사용 시)

```
IP 타깃 모드: NLB가 클라이언트 IP를 그대로 전달 (SNAT 없음)
→ 파드 보안그룹에 클라이언트 IP 대역 허용 필요
→ 또는 NLB 서브넷 IP 대역 허용 필요

Instance 타깃 모드: 노드 IP로 SNAT → 노드 보안그룹만 허용하면 됨
```

```bash
# NLB가 사용하는 서브넷 확인
aws elbv2 describe-load-balancers --names <lb-name> \
  --query 'LoadBalancers[].AvailabilityZones[].SubnetId'

# 해당 서브넷 CIDR을 파드 보안그룹 인바운드에 추가
```

---

### 사례 4: CoreDNS 응답 지연 (ndots 문제)

EKS 환경에서 자주 발생하는 DNS 조회 지연:

```yaml
# 파드 DNS 설정 최적화
spec:
  dnsConfig:
    options:
    - name: ndots
      value: "2"     # 기본값 5 → 2로 낮춰서 불필요한 search domain 조회 줄이기
    - name: timeout
      value: "2"
    - name: attempts
      value: "3"
```

---

### EKS 네트워킹 디버깅 명령어

```bash
# 파드 IP 및 노드 확인
kubectl get pods -o wide

# NLB 타깃 그룹 헬스 상태 (AWS CLI)
aws elbv2 describe-target-health \
  --target-group-arn <arn> \
  --query 'TargetHealthDescriptions[*].{Target:Target.Id,Health:TargetHealth.State,Reason:TargetHealth.Reason}'

# Endpoint 슬라이스 확인 (Ready 파드 IP 목록)
kubectl get endpointslices -l kubernetes.io/service-name=<service> -o yaml

# VPC CNI 상태 확인
kubectl get node -o custom-columns=\
'NAME:.metadata.name,\
MAX_PODS:.metadata.annotations.vpc\.amazonaws\.com/node-capacity,\
USED:.status.allocatable.pods'

# aws-node (VPC CNI) 로그
kubectl logs -n kube-system -l k8s-app=aws-node -c aws-node --tail=100

# AWS Load Balancer Controller 로그
kubectl logs -n kube-system -l app.kubernetes.io/name=aws-load-balancer-controller --tail=100
```
