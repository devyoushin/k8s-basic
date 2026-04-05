## 1. 개요 및 비유

Kubernetes 내부 DNS는 서비스 이름을 IP로 변환하는 단순 역할을 넘어, **헤드리스 서비스, SRV 레코드, ndots 정책** 등 다양한 동작 방식을 가집니다.

💡 **비유하자면 회사 내부 전화 디렉토리와 같습니다.**
"영업팀(서비스 이름)" 으로 전화하면 교환원(CoreDNS)이 실제 내선번호(파드 IP)로 연결합니다. 팀원이 바뀌어도 "영업팀" 이름으로 항상 연결됩니다.

---

## 2. DNS 레코드 구조

### 2.1 Service DNS 레코드

```
서비스 이름 패턴:
<service-name>.<namespace>.svc.<cluster-domain>

예: my-service.default.svc.cluster.local

생성되는 DNS 레코드:
A 레코드:
  my-service.default.svc.cluster.local → 10.96.100.50 (ClusterIP)

SRV 레코드 (포트 이름 있는 경우):
  _http._tcp.my-service.default.svc.cluster.local → 80 my-service.default.svc.cluster.local
  → gRPC 서비스 디스커버리, 클라이언트 사이드 로드밸런싱에 사용
```

```bash
# 파드 내부에서 DNS 조회
kubectl exec my-pod -- nslookup my-service
kubectl exec my-pod -- nslookup my-service.default.svc.cluster.local

# SRV 레코드 조회
kubectl exec my-pod -- nslookup -type=SRV _http._tcp.my-service.default

# dig으로 상세 조회
kubectl exec my-pod -- dig my-service.default.svc.cluster.local
```

### 2.2 Headless Service — 개별 파드 IP 직접 조회

일반 Service는 ClusterIP를 반환하지만, Headless Service(`clusterIP: None`)는 **파드 IP 목록을 직접 반환**합니다.

```yaml
# Headless Service 정의
apiVersion: v1
kind: Service
metadata:
  name: my-headless
spec:
  clusterIP: None      # 헤드리스
  selector:
    app: my-app
  ports:
  - port: 80
```

```
DNS 조회 결과 차이:
일반 Service:
  my-service.default.svc.cluster.local → 10.96.100.50 (ClusterIP, 1개)

Headless Service:
  my-headless.default.svc.cluster.local →
    10.244.1.2 (파드 A)
    10.244.1.3 (파드 B)
    10.244.2.5 (파드 C)
  (파드 수만큼 A 레코드 반환)

→ 클라이언트가 직접 파드 선택 가능 (클라이언트 사이드 LB)
→ gRPC, Cassandra, Kafka 등에서 활용
```

```bash
# Headless Service DNS 조회 (여러 IP 반환 확인)
kubectl exec my-pod -- nslookup my-headless.default
```

### 2.3 StatefulSet 파드의 DNS

StatefulSet + Headless Service 조합 시 **개별 파드에 안정적인 DNS**가 부여됩니다.

```
패턴: <pod-name>.<headless-service>.<namespace>.svc.<cluster-domain>

예: mysql-0.mysql-headless.default.svc.cluster.local → 10.244.1.2 (고정)
    mysql-1.mysql-headless.default.svc.cluster.local → 10.244.1.3 (고정)
    mysql-2.mysql-headless.default.svc.cluster.local → 10.244.2.5 (고정)

→ 파드가 재시작돼도 이름은 유지 (IP는 바뀔 수 있음)
→ Primary/Replica 구성에서 각 DB 인스턴스 직접 지정 가능
```

---

## 3. DNS 해석 과정 심층 분석

### 3.1 파드 내부 /etc/resolv.conf

```bash
# 파드 내부 DNS 설정 확인
kubectl exec my-pod -- cat /etc/resolv.conf
# 출력:
# nameserver 10.96.0.10        ← CoreDNS ClusterIP
# search default.svc.cluster.local svc.cluster.local cluster.local
# options ndots:5
```

### 3.2 ndots 설정과 DNS 쿼리 흐름

`ndots:5` 는 **쿼리에 점이 5개 미만이면 search 도메인을 먼저 붙여서 시도**한다는 의미입니다.

```
my-service (점 0개, 5 미만) 조회 시 순서:
1. my-service.default.svc.cluster.local  ← 첫 시도 (search 1번째)
   → 성공하면 반환

2. my-service.svc.cluster.local         ← search 2번째
3. my-service.cluster.local             ← search 3번째
4. my-service.                          ← 최종: FQDN으로 시도

google.com (점 1개, 5 미만) 조회 시:
1. google.com.default.svc.cluster.local ← 실패 (시간 낭비!)
2. google.com.svc.cluster.local         ← 실패
3. google.com.cluster.local             ← 실패
4. google.com.                          ← 성공 (4번 시도 후)

→ 외부 도메인 조회 시 불필요한 DNS 쿼리 3번 발생!
```

```bash
# DNS 쿼리 지연 측정
kubectl exec my-pod -- time nslookup google.com
# vs FQDN으로 조회 (점 끝에 붙임)
kubectl exec my-pod -- time nslookup google.com.

# DNS 쿼리 트레이스
kubectl exec my-pod -- dig +trace google.com
```

### 3.3 ndots 문제 해결

```yaml
# 방법 1: 파드에서 dnsConfig로 ndots 낮춤
spec:
  dnsConfig:
    options:
    - name: ndots
      value: "2"   # 기본 5 → 2로 낮춤
    - name: timeout
      value: "2"
    - name: attempts
      value: "3"

# 방법 2: FQDN(완전 정규화 도메인) 사용 (점으로 끝남)
# 코드에서 google.com 대신 google.com. 사용

# 방법 3: 애플리케이션 코드에서 명시적 search 도메인 분리
# 내부 서비스: my-service.default.svc.cluster.local (FQDN)
# 외부 서비스: google.com. (끝에 점)
```

---

## 4. CoreDNS 설정 심층

### 4.1 Corefile 구조

```
# CoreDNS ConfigMap 확인
kubectl get configmap coredns -n kube-system -o yaml
```

```
# 기본 Corefile 구조:
.:53 {
    errors                           # 에러 로깅
    health {
       lameduck 5s
    }
    ready                            # 준비 상태 엔드포인트
    kubernetes cluster.local in-addr.arpa ip6.arpa {
       pods insecure                 # 파드 이름 → IP 역방향 조회
       fallthrough in-addr.arpa ip6.arpa
       ttl 30                        # DNS TTL (초)
    }
    prometheus :9153                 # 메트릭 엔드포인트
    forward . /etc/resolv.conf {     # 클러스터 외부 쿼리는 노드 DNS로 포워딩
       max_concurrent 1000
    }
    cache 30                         # 응답 캐시 30초
    loop                             # DNS 루프 감지
    reload                           # Corefile 변경 자동 반영
    loadbalance                      # A 레코드 순서 랜덤화 (간단한 LB)
}
```

### 4.2 커스텀 DNS 설정 예시

```yaml
# 특정 도메인을 다른 DNS 서버로 포워딩
apiVersion: v1
kind: ConfigMap
metadata:
  name: coredns
  namespace: kube-system
data:
  Corefile: |
    .:53 {
        errors
        health
        ready
        kubernetes cluster.local in-addr.arpa ip6.arpa {
           pods insecure
           fallthrough in-addr.arpa ip6.arpa
           ttl 30
        }
        # 사내 도메인은 사내 DNS 서버로
        mycompany.internal:53 {
            forward . 10.0.0.53
        }
        prometheus :9153
        forward . 8.8.8.8 8.8.4.4   # 외부 DNS는 Google DNS로
        cache 30
        loop
        reload
        loadbalance
    }
```

---

## 5. ExternalName Service — 외부 서비스에 내부 DNS 이름 부여

```yaml
# 외부 데이터베이스에 내부 이름으로 접근
apiVersion: v1
kind: Service
metadata:
  name: prod-database   # 내부에서는 prod-database로 접근
  namespace: default
spec:
  type: ExternalName
  externalName: rds.ap-northeast-2.rds.amazonaws.com
  # → prod-database.default.svc.cluster.local 조회 시
  #    rds.ap-northeast-2.rds.amazonaws.com CNAME 반환
```

---

## 6. 트러블슈팅

* **DNS 조회 실패 (`NXDOMAIN` 또는 타임아웃):**
  ```bash
  # CoreDNS 파드 상태 확인
  kubectl get pods -n kube-system -l k8s-app=kube-dns
  kubectl logs -n kube-system -l k8s-app=kube-dns

  # CoreDNS 메트릭으로 에러율 확인
  kubectl exec -n kube-system <coredns-pod> -- \
    wget -qO- localhost:9153/metrics | grep coredns_dns_responses

  # 파드에서 직접 CoreDNS IP로 쿼리
  kubectl exec my-pod -- nslookup kubernetes.default 10.96.0.10
  ```

* **DNS 응답이 느림 (외부 도메인):**
  ```bash
  # ndots 문제인지 확인
  kubectl exec my-pod -- time nslookup external.com
  kubectl exec my-pod -- time nslookup external.com.   # FQDN
  # FQDN이 훨씬 빠르면 ndots 문제

  # CoreDNS 캐시 TTL 확인 및 조정
  # (Corefile의 cache 값 증가)
  ```

* **특정 서비스를 찾지 못함:**
  ```bash
  # 서비스 존재 여부 확인
  kubectl get svc my-service -n my-namespace

  # 파드의 네임스페이스와 서비스의 네임스페이스 확인
  # 다른 네임스페이스 서비스: <service>.<namespace>.svc.cluster.local

  # Endpoints 확인 (selector 일치 여부)
  kubectl get endpoints my-service -n my-namespace
  # Endpoints가 없으면 selector 레이블 확인
  kubectl get pods -l app=my-app -n my-namespace
  ```
