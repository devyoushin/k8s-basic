# external etcd 기반 대규모 클러스터

대규모 Kubernetes 클러스터에서는 control plane과 etcd를 분리하는 external etcd 구성을 우선 검토합니다. etcd는 Kubernetes의 모든 상태를 저장하므로, API Server와 같은 노드에서 같이 운영하면 장애 도메인과 리소스 경합이 커집니다.

## 권장 토폴로지

| 역할 | 권장 대수 | 설명 |
|------|-----------|------|
| Load Balancer | 2대 이상 또는 관리형 LB | API Server endpoint 제공 |
| Control Plane | 3대 | API Server, Scheduler, Controller Manager |
| etcd | 3대 또는 5대 | 독립 quorum 구성 |
| Worker | 워크로드 규모에 맞게 | zone/rack 분산 |

## stacked etcd와 external etcd 비교

| 방식 | 장점 | 단점 |
|------|------|------|
| stacked etcd | 설치가 단순하고 노드 수가 적음 | control plane 장애가 etcd 장애로 이어지기 쉬움 |
| external etcd | 장애 도메인 분리, 대규모 운영에 적합 | 구축/백업/인증서 관리가 복잡함 |

## etcd 설계 기준

- etcd 멤버는 홀수로 구성합니다. 일반적으로 3대, 큰 장애 도메인에서는 5대를 검토합니다.
- etcd 노드는 낮은 latency로 통신해야 합니다.
- etcd 데이터 디스크는 전용 SSD/NVMe 또는 보장된 IOPS 스토리지를 사용합니다.
- etcd peer/client 통신은 TLS로 보호합니다.
- 정기 snapshot과 restore 리허설을 운영 절차에 포함합니다.

## kubeadm external etcd 흐름

1. etcd 노드 3대를 준비합니다.
2. kubeadm으로 etcd 인증서와 static pod manifest를 생성합니다.
3. 각 etcd 노드에서 etcd cluster health를 확인합니다.
4. control plane용 kubeadm config에 external etcd endpoint와 인증서 경로를 넣습니다.
5. 첫 control plane에서 `kubeadm init --config`를 실행합니다.
6. 나머지 control plane과 worker를 join합니다.

control plane 예시 설정은 `ops/configs/kubeadm/kubeadm-external-etcd-config.yaml`에 둡니다.

## API Server Load Balancer

모든 control plane은 동일한 API Server endpoint를 사용해야 합니다.

| 항목 | 값 |
|------|-----|
| 포트 | TCP 6443 |
| health check | `/livez` 또는 TCP check |
| backend | control plane 노드 |
| 인증서 SAN | LB DNS/VIP 포함 |

## 대규모 운영 체크리스트

- control plane과 etcd를 같은 장애 도메인에 몰아두지 않습니다.
- etcd snapshot을 암호화된 외부 저장소에 보관합니다.
- `etcd_server_leader_changes_seen_total` 증가를 관측합니다.
- API Server request latency와 etcd fsync latency를 함께 봅니다.
- kubelet, CNI, CoreDNS, kube-proxy도 확장 기준을 별도로 잡습니다.
- 노드 수가 커질수록 API Priority and Fairness, admission webhook timeout, controller QPS를 점검합니다.

## 관련 문서

- `docs/components/etcd.md`
- `docs/deep-dive/etcd-backup.md`
- `docs/deep-dive/etcd-raft.md`
- `docs/deep-dive/apiserver-authn-etcd.md`

