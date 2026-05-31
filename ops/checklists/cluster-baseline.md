# Cluster Baseline Checklist

## Control Plane

- [ ] 현재 kubeconfig context를 확인했다.
- [ ] Kubernetes server/client version을 확인했다.
- [ ] kube-system Pod가 정상이다.
- [ ] CoreDNS endpoint가 정상이다.

## Nodes

- [ ] 모든 Node가 Ready 상태다.
- [ ] DiskPressure, MemoryPressure, PIDPressure가 없다.
- [ ] taint와 allocatable을 확인했다.
- [ ] metrics-server가 필요한 경우 정상 동작한다.

## Workloads

- [ ] Pending/Failed/Unknown Pod를 확인했다.
- [ ] 최근 Warning 이벤트를 확인했다.
- [ ] 주요 namespace의 resource quota를 확인했다.

## 빠른 명령

```bash
bash ops/scripts/cluster-summary.sh
bash ops/scripts/node-summary.sh
bash ops/scripts/workload-summary.sh
```
