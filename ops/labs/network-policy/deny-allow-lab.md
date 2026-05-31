# NetworkPolicy Deny/Allow Lab

NetworkPolicy가 적용되는 CNI에서 ingress 차단과 허용을 확인합니다.

## 배포

```bash
kubectl apply -f ops/manifests/deployment/nginx-deployment.yaml
kubectl apply -f ops/manifests/service/clusterip-service.yaml
```

## 기본 차단

```bash
kubectl apply -f ops/manifests/network-policy/default-deny.yaml
```

## 같은 namespace 허용

```bash
kubectl apply -f ops/manifests/network-policy/allow-same-namespace.yaml
```

## 정리

```bash
kubectl delete -f ops/manifests/network-policy/allow-same-namespace.yaml
kubectl delete -f ops/manifests/network-policy/default-deny.yaml
kubectl delete -f ops/manifests/service/clusterip-service.yaml
kubectl delete -f ops/manifests/deployment/nginx-deployment.yaml
```
