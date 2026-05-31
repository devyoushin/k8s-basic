## 1. 개요 및 비유
**Kube-Proxy**는 각 노드에서 실행되며, 쿠버네티스의 '서비스(Service)' 개념을 실제 네트워크 규칙으로 구현하는 네트워크 관리자입니다.

💡 **비유하자면 '호텔의 각 층에 배치된 내선 연결 자동 교환기'와 같습니다.**
고객이 "프론트(Service IP)"로 전화를 걸면, 각 층의 교환기(Kube-Proxy)가 현재 통화 가능한 직원(Pod IP)에게 연결해 줍니다. 손님이 1층에 있든 10층에 있든 똑같이 프론트로 연결될 수 있도록 모든 층의 교환기 설정을 똑같이 유지합니다.

## 2. 핵심 설명
* **IPVS/IPTables 관리:** 파드로 가는 가상 IP 트래픽을 실제 파드 IP로 전달하기 위해 노드의 리눅스 커널 규칙(`iptables` 또는 `ipvs`)을 실시간으로 업데이트합니다.
* **로드 밸런싱:** 서비스 뒤에 여러 파드가 있을 때, 트래픽을 골고루 분산해 주는 역할을 수행합니다.

## 3. YAML 적용 예시 (Proxy Mode 설정)
성능이 더 좋은 `ipvs` 모드를 사용하도록 설정하는 예시입니다.

```yaml
apiVersion: kubeproxy.config.k8s.io/v1alpha1
kind: KubeProxyConfiguration
mode: "ipvs" # 기본값은 iptables이나, 대규모 클러스터에선 ipvs 선호
ipvs:
  scheduler: "rr" # Round Robin 방식 사용
```

## 4. 트러블 슈팅
* **서비스 IP로는 접속이 안 되는데 파드 IP로는 접속될 때:**
  * 십중팔구 Kube-Proxy 문제입니다. 노드에서 `iptables -L -t nat` 명령어를 쳐서 해당 서비스에 대한 규칙이 생성되어 있는지 확인해야 합니다.
