# Kubernetes 클러스터 업그레이드

업그레이드는 설치보다 위험합니다. control plane, etcd, kubelet, CNI, addon의 버전 호환성을 확인하고 한 단계씩 진행해야 합니다.

## 기본 원칙

- minor version은 한 번에 하나씩 올립니다.
- control plane을 먼저 올리고, worker는 drain 후 순차적으로 올립니다.
- etcd snapshot과 애플리케이션 백업을 먼저 확보합니다.
- kubeadm으로 만든 클러스터는 kubeadm upgrade 절차를 따릅니다.
- Kubespray로 만든 클러스터는 Kubespray upgrade playbook을 기준으로 진행합니다.

## 사전 점검

```bash
kubectl get nodes -o wide
kubectl get pods -A
kubectl get --raw='/readyz?verbose'
kubectl -n kube-system get cm kubeadm-config -o yaml
```

etcd 상태와 백업을 확인합니다.

```bash
ETCDCTL_API=3 etcdctl endpoint status --write-out=table
ETCDCTL_API=3 etcdctl snapshot save snapshot.db
```

## kubeadm control plane 업그레이드

첫 control plane에서 `kubeadm`을 목표 버전으로 올립니다.

```bash
sudo apt-mark unhold kubeadm
sudo apt-get update
sudo apt-get install -y kubeadm=<TARGET_VERSION>
sudo apt-mark hold kubeadm
kubeadm version
```

업그레이드 계획을 확인합니다.

```bash
sudo kubeadm upgrade plan
sudo kubeadm upgrade apply v<TARGET_VERSION>
```

나머지 control plane은 순차적으로 진행합니다.

```bash
sudo kubeadm upgrade node
```

## kubelet/kubectl 업그레이드

각 노드를 drain한 뒤 kubelet과 kubectl을 올립니다.

```bash
kubectl drain <NODE> --ignore-daemonsets --delete-emptydir-data
sudo apt-mark unhold kubelet kubectl
sudo apt-get install -y kubelet=<TARGET_VERSION> kubectl=<TARGET_VERSION>
sudo apt-mark hold kubelet kubectl
sudo systemctl daemon-reload
sudo systemctl restart kubelet
kubectl uncordon <NODE>
```

## Kubespray 업그레이드

Kubespray는 inventory와 Kubespray release의 호환성이 중요합니다.

```bash
cd kubespray
git fetch --tags
git checkout <KUBESPRAY_RELEASE>
source .venv/bin/activate
pip install -r requirements.txt
ansible-playbook -i inventory/prod/inventory.ini upgrade-cluster.yml -b -v
```

## 업그레이드 후 확인

```bash
kubectl get nodes -o wide
kubectl get pods -A
kubectl version
kubectl -n kube-system get ds,deploy
kubectl get events -A --sort-by=.lastTimestamp
```

## 롤백 관점

Kubernetes control plane은 단순 패키지 downgrade만으로 안전하게 롤백된다고 가정하면 안 됩니다. 업그레이드 전 snapshot, manifest, 인증서, kubeadm config, addon manifest를 확보하고, 복구 시나리오를 문서화합니다.

## 점검 대상

- API Server, Scheduler, Controller Manager 정상 기동
- etcd leader 안정성
- CoreDNS 정상 응답
- CNI Pod와 NodeReady 상태
- admission webhook timeout
- Ingress/Gateway Controller
- metrics-server, Prometheus, log agent
- 주요 StatefulSet 재시작 여부

