## 1. 개요

Kubernetes 네트워킹의 핵심 원칙은 **"모든 Pod가 NAT 없이 서로 통신할 수 있어야 한다"**는 것입니다. 이를 가능하게 하는 **CNI(Container Network Interface)**의 역할과, 가상 IP를 통해 트래픽을 분산하는 **kube-proxy**의 내부 동작(iptables vs IPVS)을 심층 분석합니다.

---

## 2. 설명

### 2.1 Pod-to-Pod 통신 (CNI의 역할)

Pod가 생성될 때 CNI(예: Calico, Cilium, AWS VPC CNI)는 다음 과정을 수행합니다.

1. **네트워크 네임스페이스 생성**: Pod를 위한 격리된 네트워크 공간 확보.
2. **veth pair 생성**: 호스트(Node)와 Pod를 연결하는 가상 이더넷 케이블 연결.
3. **IP 할당**: IPAM(IP Address Management)을 통해 클러스터 내 유일한 IP 부여.
4. **라우팅 테이블 업데이트**: 다른 노드에 있는 Pod로 가는 경로를 호스트 라우팅 테이블에 등록.

### 2.2 Service Proxy: iptables vs IPVS

Service(ClusterIP)로 들어오는 요청을 실제 Pod로 전달하는 `kube-proxy`의 두 가지 모드를 비교합니다.

|**비교 항목**|**iptables (기본)**|**IPVS (고성능 추천)**|
|---|---|---|
|**알고리즘**|Sequential (순차적 규칙 확인)|Hash Table (빠른 조회)|
|**부하 분산**|Random (확률 기반)|RR, LC 등 다양한 스케줄링 지원|
|**확장성**|서비스가 많아지면 성능 저하 발생|서비스 수와 무관하게 일정한 성능 유지|

### 2.3 실무 적용 코드 (IPVS 모드 활성화 및 CNI 설정)

#### kube-proxy를 IPVS 모드로 변경

대규모 클러스터(서비스 수 1,000개 이상)에서는 IPVS 전환이 필수적입니다.

```yaml
# kube-proxy-config.yaml (ConfigMap 수정 예시)
apiVersion: kubeproxy.config.k8s.io/v1alpha1
kind: KubeProxyConfiguration
mode: "ipvs" # 기본값인 "iptables"에서 변경
ipvs:
  scheduler: "lc" # Least Connection 알고리즘 권장
---
# AWS VPC CNI 환경에서 ENI Trunking 활성화 (Pod 밀도 최적화)
# kubectl patch daemonset aws-node -n kube-system ...
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: aws-node
  namespace: kube-system
spec:
  template:
    spec:
      containers:
        - name: aws-node
          env:
            - name: ENABLE_PREFIX_DELEGATION # IP 부족 문제 해결
              value: "true"
```

### 2.4 보안(Security) 및 비용(Cost) Best Practice

- **보안 (Network Policy)**: 기본적으로 K8s는 모든 통신을 허용합니다. `default-deny-all` 정책을 세우고 필요한 통신만 허용(Whitelisting)하는 **L4/L7 Network Policy**를 적용하세요.
- **비용 (Cross-AZ Traffic)**: 다른 가용 영역(AZ) 간 통신은 데이터 전송 비용을 발생시킵니다. `topologyKeys` 또는 `Service Internal Traffic Policy`를 사용하여 동일 AZ 내의 Pod로 우선 라우팅되도록 설정하세요.
    

---

## 3. 트러블슈팅

### 3.1 DNS Look-up 지연 (ndots:5 이슈)

- **현상**: 외부 도메인 호출 시 5~10초의 지연 발생.
- **원인**: `/etc/resolv.conf`의 `ndots:5` 설정으로 인해 FQDN이 아닌 경우 여러 번의 불필요한 검색을 수행함.
- **해결**: Pod 스펙의 `dnsConfig`에서 `ndots:2`로 조정하거나, 호출 시 도메인 끝에 `.`을 붙여 FQDN으로 사용하세요.

### 3.2 IP Address Exhaustion (IP 고갈)

- **현상**: 노드에 가용 자원은 충분하나 Pod가 `Pending` 상태이며 "No IP available" 에러 발생.
- **해결**: AWS VPC CNI를 사용 중이라면 `Prefix Delegation`을 활성화하여 ENI 하나당 할당 가능한 IP 수를 획득하거나, Secondary IPv4 CIDR를 추가하세요.

---

## 4. 참고자료

- [Kubernetes Service Proxy Modes](https://www.google.com/search?q=https://kubernetes.io/docs/concepts/services-networking/service/%23virtual-ips-and-service-proxies)
- [Understanding CNI - Container Network Interface](https://github.com/containernetworking/cni)
- [A Guide to IPVS in Kubernetes](https://kubernetes.io/blog/2018/07/09/ipvs-based-in-cluster-load-balancing-deep-dive/)
    

---

## 5. TIP

- **Cilium 도입 검토**: eBPF 기반의 Cilium CNI를 사용하면 iptables를 완전히 우회하여 네트워킹 성능을 극대화하고, 강력한 보안 가시성을 얻을 수 있습니다.
- **CoreDNS 모니터링**: 클러스터 장애의 80%는 DNS에서 시작됩니다. `coredns_dns_request_duration_seconds` 메트릭에 대한 알람을 반드시 설정하세요.
- **Conntrack Table**: 대량의 트래픽이 발생하는 환경에서는 `conntrack` 테이블이 가득 차서 패킷 드랍이 발생할 수 있습니다. 노드의 커널 파라미터(`net.netfilter.nf_conntrack_max`)를 미리 튜닝해 두세요.
