# Pod Pending Runbook

## 증상

Pod가 `Pending` 상태에서 스케줄되지 않습니다.

## 확인

```bash
ns=<namespace>
pod=<pod>
kubectl describe pod "$pod" -n "$ns"
kubectl get nodes -o wide
kubectl get events -n "$ns" --sort-by=.lastTimestamp | tail -50
```

## 판단

- CPU/memory 요청량이 node allocatable보다 큰지 확인합니다.
- taint/toleration, nodeSelector, affinity 조건을 확인합니다.
- PVC binding 대기 상태인지 확인합니다.
- namespace quota 또는 limitrange에 걸리는지 확인합니다.

## 관련 문서

- `docs/deep-dive/scheduler-internals.md`
- `docs/deep-dive/scheduling-advanced.md`
- `docs/objects/taint-toleration.md`
