# Taint and Toleration Lab

Node taint와 Pod toleration 동작을 확인합니다.

## taint 추가

```bash
node=<node-name>
kubectl taint nodes "$node" dedicated=lab:NoSchedule
```

## Pod 배치 확인

```bash
kubectl apply -f ops/manifests/pod/simple-pod.yaml
kubectl describe pod simple-nginx
```

## toleration 추가 실습

`simple-pod.yaml`에 아래 toleration을 추가한 뒤 다시 적용합니다.

```yaml
tolerations:
  - key: dedicated
    operator: Equal
    value: lab
    effect: NoSchedule
```

## 정리

```bash
kubectl delete -f ops/manifests/pod/simple-pod.yaml
kubectl taint nodes "$node" dedicated=lab:NoSchedule-
```
