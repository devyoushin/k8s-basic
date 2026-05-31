# Deployment Rollout Lab

Deployment의 rolling update와 rollback을 실습합니다.

## 배포

```bash
kubectl apply -f ops/manifests/deployment/rollout-demo.yaml
kubectl rollout status deployment/rollout-demo
```

## 이미지 변경

```bash
kubectl set image deployment/rollout-demo nginx=nginx:1.26
kubectl rollout history deployment/rollout-demo
kubectl rollout status deployment/rollout-demo
```

## 롤백

```bash
kubectl rollout undo deployment/rollout-demo
kubectl rollout status deployment/rollout-demo
```

## 정리

```bash
kubectl delete -f ops/manifests/deployment/rollout-demo.yaml
```
