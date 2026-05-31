# Node NotReady Runbook

## 증상

Node가 `NotReady` 상태가 되고 Pod 스케줄링 또는 트래픽 처리가 불안정합니다.

## 확인

```bash
node=<node>
kubectl get node "$node" -o wide
kubectl describe node "$node"
kubectl get pods --all-namespaces --field-selector spec.nodeName="$node" -o wide
```

## 판단

- kubelet 상태와 container runtime 상태를 확인합니다.
- DiskPressure, MemoryPressure, PIDPressure 조건을 확인합니다.
- CNI, kube-proxy, node-local-dns 등 daemonset 상태를 확인합니다.
- 클라우드 환경이면 instance 상태와 network 상태를 확인합니다.

## 관련 문서

- `docs/components/kubelet.md`
- `docs/deep-dive/container-runtime.md`
- `docs/deep-dive/oom-eviction.md`
