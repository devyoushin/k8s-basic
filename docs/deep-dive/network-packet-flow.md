## 1. 개요 및 비유

파드 A에서 파드 B로 패킷이 전달되는 경로는 단순한 TCP/IP가 아니라 **veth → bridge → iptables/eBPF → 터널(VXLAN/BGP) → 목적지**로 이어지는 복잡한 여정입니다.

💡 **비유하자면 도시 내 택배 배송과 같습니다.**
같은 아파트(노드) 내 배송은 로비(브릿지)를 통해 바로 전달. 다른 아파트로 가는 배송은 택배 허브(라우터/터널)를 거쳐 다른 동의 로비(브릿지)로 전달됩니다.

---

## 2. 파드 네트워크 기반 구조

### 2.1 노드 내 네트워크 인터페이스

```
노드 (worker-1, 192.168.1.10)
├── eth0: 192.168.1.10/24   ← 노드 실제 NIC (외부 네트워크)
├── cni0 (브릿지): 10.244.1.1/24  ← CNI가 만든 소프트웨어 브릿지
├── veth0abc ←──────────────────────── 파드 A (10.244.1.2)의 veth 쌍
├── veth1def ←──────────────────────── 파드 B (10.244.1.3)의 veth 쌍
└── flannel.1 또는 tunl0   ← 다른 노드로 가는 터널 인터페이스

파드 A 내부:
└── eth0: 10.244.1.2/24, gw: 10.244.1.1
    (이 eth0이 실제로는 호스트의 veth0abc와 쌍을 이룸)
```

```bash
# 노드에서 CNI 브릿지 확인
ip link show type bridge
ip addr show cni0

# veth 쌍 목록 (각 파드마다 하나씩)
ip link show type veth

# 파드와 veth 매핑 확인
# 1. 파드 내부 eth0의 인터페이스 인덱스 확인
kubectl exec my-pod -- cat /sys/class/net/eth0/iflink
# 출력 예: 15

# 2. 노드에서 인덱스 15의 인터페이스 찾기
ip link | grep "^15:"
# 출력: 15: veth0abc@if3: ...
```

---

## 3. 같은 노드 내 파드 간 통신

```
파드 A (10.244.1.2) → 파드 B (10.244.1.3)

패킷 경로:
파드 A eth0 (10.244.1.2)
    │ veth 쌍으로 호스트로 탈출
    ▼
호스트 veth0abc
    │ cni0 브릿지에 연결되어 있음
    ▼
cni0 브릿지 (L2 스위칭)
    │ MAC 테이블에서 10.244.1.3 → veth1def 확인
    ▼
호스트 veth1def
    │ veth 쌍으로 파드 B로 전달
    ▼
파드 B eth0 (10.244.1.3)

→ iptables/NAT 없이 직접 L2 스위칭
→ 커널 브릿지 처리만으로 완료
```

```bash
# 브릿지 MAC 테이블 확인
bridge fdb show dev cni0

# 실시간 패킷 추적
# 파드 A에서 ping 하면서:
tcpdump -i cni0 -n 'host 10.244.1.2 or host 10.244.1.3'

# 파드 내부에서 패킷 캡처
kubectl exec -it my-pod -- tcpdump -i eth0 -n
```

---

## 4. 다른 노드 파드 간 통신 — Flannel VXLAN 방식

```
파드 A (10.244.1.2, worker-1) → 파드 C (10.244.2.5, worker-2)

worker-1에서의 경로:
파드 A eth0
    │
    ▼ veth → cni0 브릿지
cni0 (10.244.1.1)
    │ 목적지 10.244.2.5가 로컬 서브넷(10.244.1.0/24) 외부
    ▼ 라우팅 테이블 조회

ip route on worker-1:
10.244.2.0/24 via 10.244.2.0 dev flannel.1 onlink
    │
    ▼
flannel.1 (VXLAN 터널 인터페이스)
    │ 원본 L2 프레임을 UDP/8472로 캡슐화
    │ Outer: src=192.168.1.10, dst=192.168.1.20 (노드 IP)
    │ Inner: src=10.244.1.2, dst=10.244.2.5 (파드 IP)
    ▼
eth0 (노드 NIC) → 물리 네트워크

worker-2에서의 경로:
eth0 수신 (UDP 8472 포트)
    │
    ▼
flannel.1 (VXLAN 디캡슐화)
    │ Inner 패킷 복원: 10.244.1.2 → 10.244.2.5
    ▼
라우팅 → cni0 브릿지
    │
    ▼ veth → 파드 C eth0 (10.244.2.5)
```

```bash
# VXLAN 터널 인터페이스 확인
ip link show flannel.1
ip -d link show flannel.1  # VXLAN VNI, 포트 확인

# FDB (Forwarding Database): 어느 노드에 어떤 파드 서브넷이 있는지
bridge fdb show dev flannel.1
# 출력 예:
# aa:bb:cc:dd:ee:ff dev flannel.1 dst 192.168.1.20 self permanent
# (worker-2의 MAC → worker-2 노드 IP)

# 실제 VXLAN 캡슐화 패킷 캡처
tcpdump -i eth0 -n 'udp port 8472'
```

---

## 5. 다른 노드 파드 간 통신 — Calico BGP 방식

```
Calico BGP (오버레이 없음, 순수 L3 라우팅):

각 노드가 BGP 라우터가 되어 파드 서브넷 경로 광고
worker-1 (10.244.1.0/24) ←BGP→ worker-2 (10.244.2.0/24)

worker-1 라우팅 테이블 (Calico BGP 적용 후):
10.244.2.0/24 via 192.168.1.20 dev eth0
(VXLAN 없이 직접 라우팅!)

패킷 경로:
파드 A → veth → 라우팅 테이블 → eth0 → 물리 네트워크
→ worker-2 eth0 → 라우팅 → veth → 파드 C

장점: VXLAN 캡슐화/디캡슐화 오버헤드 없음, 더 빠름
조건: 모든 노드가 같은 L2 네트워크에 있거나, BGP를 지원하는 라우터가 있어야 함
```

```bash
# Calico BGP 피어 상태 확인
calicoctl node status
# BGP 세션이 Established 상태인지 확인

# Calico가 학습한 라우팅 테이블
ip route | grep bird
# 또는
ip route show proto bird

# BGP 라우팅 상세 확인
calicoctl get bgpPeer
```

---

## 6. Service를 통한 패킷 흐름 (iptables 모드)

```
파드 A (10.244.1.2) → Service ClusterIP (10.96.0.1:80)
                    → 실제 파드 C (10.244.2.5:8080)

iptables DNAT 흐름:

파드 A에서 패킷 발생:
  src=10.244.1.2, dst=10.96.0.1:80

커널 Netfilter (iptables):
  PREROUTING → KUBE-SERVICES 체인
    │
    ├─ KUBE-SVC-XXXXXX (Service 10.96.0.1:80 매칭)
    │    │
    │    ├─ 33% 확률: KUBE-SEP-AAA (파드 A, 10.244.1.2)
    │    ├─ 33% 확률: KUBE-SEP-BBB (파드 B, 10.244.1.3)
    │    └─ 33% 확률: KUBE-SEP-CCC (파드 C, 10.244.2.5)
    │
    └─ DNAT: dst를 10.96.0.1:80 → 10.244.2.5:8080 으로 변경

변환된 패킷:
  src=10.244.1.2, dst=10.244.2.5:8080
  → 이후 일반 파드 간 라우팅으로 처리
```

```bash
# kube-proxy가 생성한 iptables 규칙 확인
iptables-save | grep KUBE-SERVICES | head -20

# 특정 Service의 iptables 규칙
iptables-save | grep "10.96.0.1"

# Service → Endpoints 매핑 확인
kubectl get endpoints my-service -o yaml

# conntrack 테이블 (현재 연결 추적)
conntrack -L | grep 10.96.0.1
# DNAT 변환이 기록된 항목 확인
```

---

## 7. eBPF 기반 네트워킹 (Cilium)

```
iptables의 한계:
- 파드 수 증가 → iptables 규칙 수 폭발적 증가
- 규칙 추가/삭제 시 전체 테이블 잠금 (O(n) 성능)
- 10,000개 서비스 = 수십만 개 iptables 규칙

Cilium (eBPF):
- iptables 규칙 없이 커널 내 eBPF 프로그램으로 처리
- O(1) 해시맵 조회
- XDP(eXpress Data Path): NIC 드라이버 레벨에서 처리

eBPF 패킷 처리 위치:
NIC 수신 → [XDP hook] → [TC ingress hook] → 네트워킹 스택
                                               → [TC egress hook] → NIC 송신
```

```bash
# Cilium 설치된 경우 eBPF 맵 확인
cilium status

# eBPF 서비스 맵 확인 (iptables 대체)
cilium service list

# 특정 파드의 eBPF 정책 확인
cilium endpoint list
cilium endpoint get <endpoint-id>

# 패킷 드롭 모니터링
cilium monitor --type drop
```

---

## 8. 트러블슈팅

* **파드 간 통신 안 됨:**
  ```bash
  # NetworkPolicy 차단 여부 확인
  kubectl get networkpolicy -A

  # 실제 경로 추적
  kubectl exec my-pod -- traceroute 10.244.2.5
  # 또는
  kubectl exec my-pod -- curl -v telnet://10.244.2.5:8080

  # iptables REJECT/DROP 규칙 확인
  iptables -L -n | grep DROP
  ```

* **Service ClusterIP로 접근 안 됨:**
  ```bash
  # kube-proxy 정상 동작 확인
  kubectl get pods -n kube-system | grep kube-proxy
  kubectl logs -n kube-system <kube-proxy-pod>

  # iptables 규칙에 Service가 있는지 확인
  iptables-save | grep <ClusterIP>

  # Endpoints가 존재하는지 확인 (파드가 Ready 상태여야)
  kubectl get endpoints <service-name>
  ```

* **노드 간 파드 통신 안 됨 (Flannel VXLAN):**
  ```bash
  # flannel.1 인터페이스 UP 상태 확인
  ip link show flannel.1

  # flannel FDB 테이블에 대상 노드가 있는지
  bridge fdb show dev flannel.1 | grep <상대노드IP>

  # UDP 8472 포트 방화벽 허용 여부
  iptables -L INPUT -n | grep 8472
  ```
