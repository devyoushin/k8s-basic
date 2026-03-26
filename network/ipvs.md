## 1. 개요

쿠버네티스의 `kube-proxy`는 기본적으로 `iptables` 모드를 사용하지만, 대규모 클러스터(서비스 개수 1,000개 이상)에서는 성능 저하가 발생합니다. **IPVS(IP Virtual Server)** 모드는 리눅스 커널 레벨의 L4 로드밸런싱 기술을 활용하여 성능 최적화와 다양한 스케줄링 알고리즘을 제공합니다. 본 문서에서는 IPVS의 동작 원리와 CNI(특히 Calico/Cilium)와의 연계 방안을 다룹니다.

## 2. 설명

### 2.1 Why IPVS?

`iptables`는 체인 방식의 순차적 탐색($O(n)$)을 수행하므로 서비스가 많아질수록 지연 시간이 증가합니다. 반면 `IPVS`는 해시 테이블($O(1)$)을 사용하여 서비스 규모에 관계없이 일정한 성능을 유지합니다.

### 2.2 실무 적용 코드

#### A. Kube-proxy IPVS 설정 

`kube-proxy` 설정에서 모드를 `ipvs`로 변경하고 필요한 커널 모듈을 로드해야 합니다.

```yaml
# kube-proxy-config.yaml
apiVersion: kubeproxy.config.k8s.io/v1alpha1
kind: KubeProxyConfiguration
mode: "ipvs"
ipvs:
  scheduler: "rr" # Round Robin
  strictARP: true # CNI(MetalLB, Cilium 등)와 호환성을 위해 필수
```

#### B. 필수 커널 모듈 로드 

IPVS를 사용하려면 노드 부팅 시 커널 모듈이 로드되어 있어야 합니다.

```hcl
# main.tf (EKS/Self-managed Node Group UserData 부분)
resource "aws_instance" "k8s_node" {
  # ... 생략 ...
  user_data = <<-EOF
              #!/bin/bash
              modprobe -- ip_vs
              modprobe -- ip_vs_rr
              modprobe -- ip_vs_wrr
              modprobe -- ip_vs_sh
              modprobe -- nf_conntrack
              
              # persist modules
              cut -d' ' -f1 /proc/modules | grep -E 'ip_vs|nf_conntrack' > /etc/modules-load.d/ipvs.conf
              EOF
}
```

### 2.3 보안(Security) & 비용(Cost) Best Practice

- **보안:** IPVS 모드에서도 `NetworkPolicy`가 정상 작동하는지 확인해야 합니다. Calico 같은 CNI는 IPVS 모드에서도 자체적으로 `iptables`나 `eBPF`를 써서 정책을 적용하지만, 설정이 꼬이면 정책이 무시될 수 있습니다.
- **비용:** IPVS 도입 자체로 비용이 발생하진 않지만, **CPU 오버헤드 감소**를 통해 인스턴스 패밀리 사양을 낮추거나(Consolidation), 대규모 트래픽 처리 시 응답 속도 개선으로 인한 인프라 효율성을 높일 수 있습니다.
    

## 3. 트러블슈팅 및 모니터링

### 3.1 모니터링 및 알람 전략

IPVS 상태를 감시하기 위해 `IPVS Metrics Exporter`를 사용하거나 `node_exporter`의 IPVS 컬렉터를 활성화합니다.

- **핵심 지표:** `node_ipvs_connections_active` (현재 활성 연결 수)
- **알람 임계치:**
    - **Warning:** 특정 서비스의 활성 연결이 평소 대비 200% 급증 시 (DDoS 또는 App 로직 오류 의심)
    - **Critical:** IPVS 커널 모듈 미로드로 인한 `kube-proxy` CrashLoopBackOff 발생 시
        

### 3.2 자주 발생하는 문제 (Troubleshooting)

1. **Strict ARP 문제:** * **현상:** MetalLB나 특정 CNI 사용 시 서비스 IP로 통신이 간헐적으로 안 됨.
    - **해결:** `kube-proxy` 설정에서 `strictARP: true`로 설정하여 커널이 ARP 요청에 대해 보다 엄격하게 반응하도록 해야 합니다.
        
2. **커널 모듈 부재:** * **현상:** `kube-proxy` 로그에 `Can't use ipvs mode, falling back to iptables` 출력.
    - **해결:** 노드 레벨에서 `lsmod | grep ip_vs` 명령어로 모듈 로드 여부를 확인하세요.
        

## 4. 참고자료

- [Kubernetes 공식 문서: Service Proxy Modes](https://www.google.com/search?q=https://kubernetes.io/docs/concepts/services-networking/service-proxy-setup/%23ipvs-proxy-mode)
- [IPVS 기반 로드밸런싱 알고리즘 정리](http://www.linuxvirtualserver.org/docs/scheduling.html)
    

## TIP

- **CNI 선택:** 최근 트렌드는 `eBPF`를 사용하는 **Cilium**입니다. Cilium을 사용하면 `kube-proxy`를 아예 제거(Kube-proxy Replacement)하고 IPVS보다 더 빠른 성능을 낼 수 있습니다.
- **IPVS 스케줄러:** 보통 `rr`(Round Robin)을 쓰지만, 서버 성능이 제각각인 하이브리드 환경이라면 `lc`(Least Connection)를 검토해 보세요.
