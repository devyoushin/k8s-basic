## 1. 개요 및 비유

NodeLocal DNSCache는 각 노드에 DNS 캐시 에이전트를 두어 CoreDNS까지 가지 않고 로컬에서 DNS 쿼리를 처리합니다.

💡 **비유하자면 지사 직원이 본사(CoreDNS)에 매번 전화하는 대신, 자기 책상 위 메모장(로컬 캐시)을 먼저 확인하는 것과 같습니다.**
자주 묻는 것은 메모장에서 즉시 답하고, 새로운 질문만 본사에 전화합니다.

---

## 2. 기존 DNS 구조의 문제점

### 2.1 CoreDNS 병목과 conntrack 경쟁

```
일반 파드 DNS 쿼리 흐름:
파드 → iptables DNAT → CoreDNS ClusterIP(10.96.0.10)
    → 실제 CoreDNS 파드 (랜덤 선택)

문제점:
┌──────────────────────────────────────────────────────────┐
│ 1. conntrack 레이스 컨디션                               │
│    UDP DNS 쿼리는 connectionless                         │
│    여러 파드가 동시에 같은 5-tuple로 쿼리 시              │
│    conntrack 테이블 충돌 → DNS 쿼리 드롭                  │
│    → 5초 타임아웃 후 재시도 → 앱 응답 지연               │
│                                                          │
│ 2. CoreDNS 스케일 한계                                   │
│    수천 개 파드의 DNS 쿼리가 몇 개의 CoreDNS로 집중       │
│    CoreDNS HPA로도 해결 어려움 (iptables LB 불균형)      │
│                                                          │
│ 3. 캐시 낭비                                             │
│    모든 노드의 파드가 같은 쿼리를 CoreDNS에 전달          │
│    CoreDNS 캐시가 있어도 노드간 캐시 공유 안 됨           │
└──────────────────────────────────────────────────────────┘
```

---

## 3. NodeLocal DNSCache 동작 원리

```
NodeLocal DNSCache 설치 후 흐름:
파드 → 링크-로컬 IP (169.254.20.10) → 노드 내 cache 에이전트
           (iptables가 아닌 직접 연결)
                    │
                    ├─[캐시 히트] → 즉시 반환
                    │
                    └─[캐시 미스] → CoreDNS (TCP 연결, conntrack 안정적)

장점:
- conntrack 경쟁 없음 (링크-로컬 IP → iptables 우회)
- 노드별 캐시 → CoreDNS 부하 감소
- 캐시 히트 시 <1ms 응답 (CoreDNS는 ~5ms)
- CoreDNS → cache 에이전트 구간은 TCP 사용 (안정적)
```

```
NodeLocal DNSCache 아키텍처:
┌─────────────────────────────────────────────┐
│ 노드 (worker-1)                              │
│                                             │
│  파드 A   파드 B   파드 C                    │
│    │        │        │                      │
│    └────────┴────────┘                      │
│              │ UDP to 169.254.20.10:53       │
│              ▼                              │
│    ┌─────────────────────┐                  │
│    │ node-local-dns 파드  │ ← DaemonSet      │
│    │ (coredns 기반)       │                  │
│    │ 캐시 TTL: 30초       │                  │
│    └──────────┬──────────┘                  │
│               │ 캐시 미스 시 TCP             │
└───────────────┼─────────────────────────────┘
                │
                ▼
         CoreDNS (10.96.0.10)
```

---

## 4. 설치 및 설정

```bash
# NodeLocal DNSCache 설치 (Kubernetes 1.18+ 기본 제공)
# 공식 매니페스트 다운로드 및 적용
curl -O https://raw.githubusercontent.com/kubernetes/kubernetes/master/cluster/addons/dns/nodelocaldns/nodelocaldns.yaml

# 변수 치환
# PILLAR__LOCAL__DNS: 169.254.20.10 (링크-로컬 IP)
# PILLAR__DNS__DOMAIN: cluster.local
# PILLAR__DNS__SERVER: CoreDNS ClusterIP (보통 10.96.0.10)
sed -i 's/__PILLAR__LOCAL__DNS__/169.254.20.10/g' nodelocaldns.yaml
sed -i 's/__PILLAR__DNS__DOMAIN__/cluster.local/g' nodelocaldns.yaml
sed -i 's/__PILLAR__DNS__SERVER__/10.96.0.10/g' nodelocaldns.yaml

kubectl apply -f nodelocaldns.yaml

# 설치 확인 (모든 노드에 DaemonSet)
kubectl get pods -n kube-system -l k8s-app=node-local-dns
```

### 4.1 Corefile 설정

```
# NodeLocal DNSCache의 Corefile
cluster.local:53 {                       # 클러스터 내부 도메인
    errors
    cache {
        success 9984 30                  # 최대 9984개, 30초 TTL
        denial 9984 5                    # NXDOMAIN 캐시 5초
    }
    reload
    loop
    bind 169.254.20.10                   # 링크-로컬 IP에서 수신
    forward . 10.96.0.10 {              # CoreDNS로 포워딩
        force_tcp                        # 반드시 TCP 사용 (conntrack 안정성)
    }
    prometheus :9253
    health 169.254.20.10:8080
}

.:53 {                                   # 외부 도메인
    errors
    cache 30
    reload
    loop
    bind 169.254.20.10
    forward . /etc/resolv.conf {         # 노드의 업스트림 DNS로
        force_tcp
    }
    prometheus :9253
}
```

### 4.2 파드에서 NodeLocal DNSCache 사용 설정

```
kubelet에 다음 플래그 추가 필요:
--cluster-dns=169.254.20.10    (NodeLocal DNS 주소)
--cluster-domain=cluster.local

→ 이후 생성되는 파드의 /etc/resolv.conf:
nameserver 169.254.20.10       (변경됨)
search default.svc.cluster.local svc.cluster.local cluster.local
options ndots:5
```

```bash
# 기존 파드에서 DNS 서버 확인
kubectl exec my-pod -- cat /etc/resolv.conf
# nameserver 169.254.20.10 이면 NodeLocal 사용 중

# NodeLocal DNSCache 상태 확인
kubectl exec -n kube-system node-local-dns-xxxxx -- \
  wget -qO- http://169.254.20.10:8080/health

# 캐시 통계 확인 (Prometheus 메트릭)
kubectl exec -n kube-system node-local-dns-xxxxx -- \
  wget -qO- http://169.254.20.10:9253/metrics | grep coredns_cache
```

---

## 5. 성능 개선 측정

```bash
# NodeLocal DNSCache 적용 전후 DNS 응답 시간 비교

# 적용 전 (CoreDNS 직접)
kubectl exec my-pod -- bash -c '
for i in {1..100}; do
  time nslookup kubernetes.default 2>&1 | grep real
done' | awk '{sum+=$2} END {print "평균:" sum/NR "초"}'

# 적용 후 (NodeLocal 캐시)
kubectl exec my-pod -- bash -c '
for i in {1..100}; do
  time nslookup kubernetes.default 2>&1 | grep real
done' | awk '{sum+=$2} END {print "평균:" sum/NR "초"}'

# 캐시 히트율 확인
kubectl exec -n kube-system node-local-dns-xxxxx -- \
  wget -qO- http://169.254.20.10:9253/metrics | \
  grep -E "coredns_cache_hits_total|coredns_cache_misses_total"
```

---

## 6. 트러블슈팅

* **파드가 DNS 쿼리를 CoreDNS로 직접 보냄 (NodeLocal 우회):**
  ```bash
  # kubelet 설정 확인
  cat /var/lib/kubelet/config.yaml | grep clusterDNS
  # 169.254.20.10 이어야 함

  # 파드 재생성 필요 (기존 파드는 이전 설정 유지)
  kubectl rollout restart deployment my-app
  ```

* **NodeLocal DNS가 특정 노드에서 시작 안 됨:**
  ```bash
  kubectl describe pod -n kube-system node-local-dns-xxxxx | grep -A5 Events

  # 링크-로컬 IP 이미 사용 중인 경우
  ip addr show | grep 169.254.20.10
  # → 다른 프로세스가 사용 중이면 충돌

  # iptables 규칙 확인
  iptables -t raw -L OUTPUT -n | grep 169.254.20.10
  ```

* **DNS 캐시 TTL 동안 오래된 IP 반환:**
  ```bash
  # 캐시 강제 플러시 (NodeLocal 파드 재시작)
  kubectl rollout restart daemonset node-local-dns -n kube-system

  # 또는 특정 도메인의 TTL 확인
  kubectl exec my-pod -- dig kubernetes.default.svc.cluster.local | grep TTL
  ```
