# Kubespray 기반 설치

Kubespray는 Ansible로 Kubernetes 클러스터를 구성하는 도구입니다. 내부적으로 kubeadm 기반 부트스트랩을 사용하며, inventory와 group_vars로 container runtime, CNI, control plane, etcd, addon 구성을 표준화할 수 있습니다.

## 언제 Kubespray를 쓰는가

- 여러 환경에 동일한 Kubernetes 설치 절차를 반복해야 한다.
- control plane, worker, etcd 노드 구성을 inventory로 관리하고 싶다.
- kubeadm 직접 실행보다 Ansible 기반 자동화가 필요하다.
- 설치뿐 아니라 upgrade, scale-out, reset 절차도 같은 도구로 관리하고 싶다.

## 기본 흐름

```bash
git clone https://github.com/kubernetes-sigs/kubespray.git
cd kubespray
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
cp -r inventory/sample inventory/prod
```

inventory 예시는 이 레포의 `ops/configs/kubespray/inventory.ini`를 참고합니다.

```bash
ansible-playbook -i inventory/prod/inventory.ini cluster.yml -b -v
```

## inventory 핵심 그룹

| 그룹 | 역할 |
|------|------|
| `kube_control_plane` | API Server, Scheduler, Controller Manager |
| `etcd` | etcd 멤버 |
| `kube_node` | worker 노드 |
| `k8s_cluster:children` | control plane과 worker 전체 |
| `calico_rr` | Calico route reflector 사용 시 |

## 주요 group_vars

| 설정 | 설명 |
|------|------|
| `kube_version` | 설치할 Kubernetes 버전 |
| `container_manager` | `containerd` 등 런타임 |
| `kube_network_plugin` | `calico`, `cilium` 등 CNI |
| `etcd_deployment_type` | etcd 배포 방식 |
| `supplementary_addresses_in_ssl_keys` | API Server 인증서에 넣을 LB/VIP 주소 |

## external etcd 구성

대규모 클러스터에서는 `etcd` 그룹을 control plane과 분리합니다.

```ini
[kube_control_plane]
cp-1
cp-2
cp-3

[etcd]
etcd-1
etcd-2
etcd-3

[kube_node]
worker-1
worker-2
worker-3
```

## 설치 후 확인

```bash
kubectl get nodes -o wide
kubectl get pods -A
kubectl -n kube-system get cm kubeadm-config -o yaml
```

## 운영 주의점

- Kubespray repo 버전과 Kubernetes target version의 호환성을 먼저 확인합니다.
- inventory는 Git으로 관리하되 SSH 키, bootstrap token, 인증서 등 민감 값은 제외합니다.
- 운영 전 `facts`, `group_vars`, `host_vars` 변경 이력을 명확히 남깁니다.
- Kubespray로 만든 클러스터는 upgrade도 Kubespray 기준 절차로 진행하는 것이 일관성이 좋습니다.

