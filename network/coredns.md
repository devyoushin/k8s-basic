## 1. 개요 및 비유
**CoreDNS**는 쿠버네티스 클러스터 내부의 DNS 서버입니다. 파드가 서비스 이름(`backend-svc`)으로 다른 서비스를 찾을 수 있도록 이름을 IP로 변환해줍니다.

💡 **비유하자면 '사내 전화번호부 안내원'과 같습니다.**
"백엔드 팀 연결해주세요(backend-svc)"라고 말하면 안내원(CoreDNS)이 전화번호부(DNS 레코드)를 뒤져 실제 내선번호(ClusterIP)를 알려줍니다. 안내원이 자리를 비우면 사내 전화가 전부 먹통이 됩니다 — 클러스터 장애의 80%가 DNS에서 시작하는 이유입니다.

## 2. 핵심 설명

### DNS 이름 규칙
쿠버네티스의 DNS 이름 구조입니다.

| 리소스 | DNS 이름 |
|---|---|
| Service | `<서비스명>.<네임스페이스>.svc.cluster.local` |
| Pod | `<파드IP 하이픈>..<네임스페이스>.pod.cluster.local` |

같은 네임스페이스 안에서는 서비스명만으로 접근 가능합니다. (`backend-svc`)
다른 네임스페이스의 서비스는 `backend-svc.production` 또는 전체 FQDN이 필요합니다.

### ndots:5 문제
파드의 `/etc/resolv.conf` 기본 설정은 `ndots:5`입니다. 점(`.`)이 5개 미만인 도메인은 FQDN으로 간주하지 않고, 아래 search 도메인들을 순서대로 붙여서 먼저 조회합니다.

```
search default.svc.cluster.local svc.cluster.local cluster.local
ndots:5
```

`api.example.com` (점 2개) 을 조회하면 실제로 다음 순서로 DNS 쿼리를 보냅니다:
1. `api.example.com.default.svc.cluster.local` ← 실패
2. `api.example.com.svc.cluster.local` ← 실패
3. `api.example.com.cluster.local` ← 실패
4. `api.example.com` ← 성공 (4번 만에)

이 불필요한 조회들이 외부 API 호출 시 수백 ms의 레이턴시를 유발합니다.

### CoreDNS 아키텍처
CoreDNS는 `kube-system` 네임스페이스에 Deployment로 배포되며, ConfigMap(`coredns`)으로 설정을 관리합니다.

## 3. YAML 적용 예시

### ndots 문제 해결 (파드 단위)
```yaml
spec:
  containers:
  - name: app
    image: my-app:1.0
  dnsConfig:
    options:
    - name: ndots
      value: "2"    # 점이 2개 미만일 때만 search 도메인 적용
```

### CoreDNS ConfigMap 커스터마이징
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: coredns
  namespace: kube-system
data:
  Corefile: |
    .:53 {
        errors
        health {
          lameduck 5s
        }
        ready
        kubernetes cluster.local in-addr.arpa ip6.arpa {
          pods insecure
          fallthrough in-addr.arpa ip6.arpa
          ttl 30
        }
        # 특정 도메인을 내부 DNS로 포워딩 (사내 DNS 연동)
        forward corp.example.com 10.0.0.53
        # 나머지는 외부 DNS로
        forward . 8.8.8.8 8.8.4.4 {
          max_concurrent 1000
        }
        cache 30
        loop
        reload
        loadbalance
    }
```

### CoreDNS HPA 설정 (대규모 클러스터)
```yaml
# CoreDNS Deployment에 HPA 적용
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: coredns
  namespace: kube-system
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: coredns
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

**DNS 디버깅 명령어:**
```bash
# 파드 내부에서 DNS 조회 테스트
kubectl run dns-test --image=busybox --rm -it --restart=Never -- nslookup kubernetes.default

# CoreDNS 로그 확인
kubectl logs -n kube-system -l k8s-app=kube-dns --tail=50

# CoreDNS 파드 상태 확인
kubectl get pods -n kube-system -l k8s-app=kube-dns

# DNS 쿼리 레이턴시 확인 (CoreDNS 메트릭)
kubectl top pod -n kube-system -l k8s-app=kube-dns
```

## 4. 트러블 슈팅

* **서비스 이름으로 접속이 안 됨 (Name or service not known):**
  1. CoreDNS 파드가 Running인지 확인: `kubectl get pods -n kube-system -l k8s-app=kube-dns`
  2. 파드 내부에서 직접 DNS 조회 테스트: `nslookup <서비스명>`
  3. NetworkPolicy가 파드 → CoreDNS(UDP/TCP 53) 트래픽을 차단하고 있는지 확인

* **외부 도메인 호출 시 간헐적 5초 지연:**
  * `ndots:5` 문제입니다. 파드 스펙에 `dnsConfig.options.ndots: "2"` 를 설정하거나, 호출하는 도메인 끝에 `.`을 붙여 FQDN으로 사용하세요. (`api.example.com.`)

* **CoreDNS CPU 사용률 급등 (대규모 클러스터):**
  * DNS 쿼리가 집중되는 것입니다. 다음을 순서대로 검토하세요.
    1. `ndots` 값 낮추기 → 불필요한 쿼리 자체를 줄임
    2. CoreDNS `cache` 플러그인 TTL 늘리기 → 반복 쿼리 감소
    3. CoreDNS 레플리카 수 늘리기 (또는 HPA 적용)
    4. `NodeLocal DNSCache` 도입 → 각 노드에 DNS 캐시를 두어 CoreDNS 부하 분산

* **`SERVFAIL` 응답:**
  * CoreDNS가 업스트림 DNS에 도달하지 못하는 것입니다. 노드의 아웃바운드 DNS(포트 53) 방화벽 규칙과 CoreDNS의 `forward` 설정을 확인하세요.
