# Kubernetes 설치와 업그레이드

Kubernetes 설치는 단순히 `kubeadm init`을 실행하는 문제가 아니라, control plane 고가용성, etcd 배치, CNI, 인증서, 업그레이드 전략까지 함께 정하는 작업입니다. 이 디렉터리는 운영 클러스터를 직접 구축할 때 필요한 설치 방식을 정리합니다.

## 빠른 선택 기준

| 목적 | 권장 방식 | 설명 |
|------|-----------|------|
| 학습/소규모 운영 | `kubeadm` 직접 설치 | Kubernetes 동작 원리를 이해하기 좋고 절차가 명확함 |
| 반복 가능한 운영 구축 | Kubespray | Ansible inventory로 노드/네트워크/런타임 설정을 표준화 |
| 대규모/중요 서비스 | external etcd + HA control plane | etcd quorum과 control plane 장애 도메인을 분리 |
| 운영 중 버전 변경 | upgrade 절차 | control plane, kubelet, CNI, addon 순서로 단계적 진행 |

## 문서 순서

1. `kubeadm.md` - kubeadm 기반 기본 설치 흐름
2. `kubespray.md` - Kubespray로 kubeadm 기반 클러스터 자동화
3. `external-etcd-large-cluster.md` - etcd 분리형 대규모 클러스터 설계
4. `upgrade.md` - kubeadm/Kubespray 업그레이드 절차와 체크리스트

## 운영 기준 결론

- 단일 control plane은 학습용으로만 사용합니다.
- 운영은 최소 3대 control plane과 3대 etcd를 기준으로 설계합니다.
- 대규모 클러스터는 stacked etcd보다 external etcd를 우선 검토합니다.
- Kubespray는 설치 편의성이 높지만, inventory와 group_vars를 운영 표준으로 관리해야 합니다.
- 업그레이드는 한 번에 여러 minor version을 건너뛰지 않고, minor version 단위로 진행합니다.

## 참고 기준

- Kubernetes 공식 kubeadm 설치 문서
- Kubernetes 공식 kubeadm HA/external etcd 문서
- Kubernetes 공식 kubeadm upgrade 문서
- Kubespray 공식 upgrade 문서

