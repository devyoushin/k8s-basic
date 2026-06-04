# kubeadm 기반 Kubernetes 설치

`kubeadm`은 Kubernetes 클러스터 부트스트랩 도구입니다. control plane static pod, 인증서, kubeconfig, kubelet bootstrap, join token을 표준 방식으로 구성합니다.

## 구성 전제

| 항목 | 기준 |
|------|------|
| OS | Ubuntu/RHEL 계열 Linux |
| Container Runtime | containerd |
| Swap | 비활성화 |
| CNI | Calico, Cilium 등 별도 설치 |
| Control Plane | 운영은 3대 이상 권장 |
| API Server Endpoint | LB VIP 또는 L4 Load Balancer |

## 노드 공통 준비

```bash
sudo swapoff -a
sudo modprobe overlay
sudo modprobe br_netfilter
```

```bash
cat <<EOF | sudo tee /etc/sysctl.d/99-kubernetes.conf
net.bridge.bridge-nf-call-iptables = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward = 1
EOF
sudo sysctl --system
```

containerd를 설치하고 systemd cgroup을 사용하도록 맞춥니다.

```bash
sudo containerd config default | sudo tee /etc/containerd/config.toml
sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
sudo systemctl restart containerd
sudo systemctl enable containerd
```

## kubeadm/kubelet/kubectl 설치

Kubernetes 패키지 저장소는 minor version별로 분리됩니다. 설치하려는 클러스터 minor version에 맞는 저장소를 사용합니다.

```bash
sudo apt-get update
sudo apt-get install -y apt-transport-https ca-certificates curl gpg
sudo mkdir -p /etc/apt/keyrings
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.36/deb/Release.key \
  | sudo gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.36/deb/ /' \
  | sudo tee /etc/apt/sources.list.d/kubernetes.list
sudo apt-get update
sudo apt-get install -y kubelet kubeadm kubectl
sudo apt-mark hold kubelet kubeadm kubectl
```

## 첫 control plane 초기화

예시 설정은 `ops/configs/kubeadm/kubeadm-ha-config.yaml`에 둡니다.

```bash
sudo kubeadm init --config ops/configs/kubeadm/kubeadm-ha-config.yaml --upload-certs
```

kubectl 설정을 복사합니다.

```bash
mkdir -p "$HOME/.kube"
sudo cp /etc/kubernetes/admin.conf "$HOME/.kube/config"
sudo chown "$(id -u):$(id -g)" "$HOME/.kube/config"
```

## CNI 설치

`kubeadm init` 후에는 Pod 네트워크가 없기 때문에 CoreDNS가 정상 기동하지 않을 수 있습니다. Calico, Cilium 등 운영 표준 CNI를 설치합니다.

```bash
kubectl get nodes
kubectl get pods -n kube-system
```

## control plane 추가

첫 control plane에서 출력된 `kubeadm join` 명령을 사용합니다. 인증서 키가 만료되었다면 다시 업로드합니다.

```bash
sudo kubeadm init phase upload-certs --upload-certs
```

## worker 추가

```bash
sudo kubeadm token create --print-join-command
```

출력된 join 명령을 worker 노드에서 실행합니다.

## 설치 후 확인

```bash
kubectl get nodes -o wide
kubectl get pods -A
kubectl -n kube-system get endpoints kube-controller-manager kube-scheduler
kubectl cluster-info
```

## 운영 주의점

- API Server endpoint는 control plane 노드 IP가 아니라 LB 주소를 사용합니다.
- kubelet과 containerd의 cgroup driver를 맞춥니다.
- 인증서 만료일을 정기적으로 확인합니다.
- etcd snapshot과 restore 절차를 설치 직후 검증합니다.
- 단일 control plane은 장애 복구 훈련용 또는 학습용으로만 사용합니다.

