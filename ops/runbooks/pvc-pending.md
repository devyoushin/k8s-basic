# PVC Pending Runbook

## 증상

PVC가 `Pending` 상태로 남고 Pod가 볼륨을 mount하지 못합니다.

## 확인

```bash
ns=<namespace>
pvc=<pvc>
kubectl get pvc "$pvc" -n "$ns"
kubectl describe pvc "$pvc" -n "$ns"
kubectl get storageclass
kubectl get pv
```

## 판단

- default StorageClass가 있는지 확인합니다.
- 요청한 access mode를 지원하는지 확인합니다.
- 동적 provisioner가 정상인지 확인합니다.
- WaitForFirstConsumer 모드라면 Pod scheduling 조건을 같이 확인합니다.

## 관련 문서

- `docs/objects/pv-pvc.md`
- `docs/deep-dive/storage-csi.md`
