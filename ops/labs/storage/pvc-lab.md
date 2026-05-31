# PVC Lab

StorageClass와 PVC binding 흐름을 확인합니다.

## 확인

```bash
kubectl get storageclass
kubectl get pv,pvc --all-namespaces
```

## PVC 예시

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: demo-pvc
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
```

## 확인 포인트

- default StorageClass가 있는가?
- PVC가 Pending이면 provisioner, access mode, zone을 확인할 수 있는가?
- Pod가 PVC를 mount할 때 node affinity 문제가 생길 수 있음을 이해하는가?
