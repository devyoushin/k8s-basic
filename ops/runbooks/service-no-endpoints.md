# Service No Endpoints Runbook

## 증상

Service는 존재하지만 endpoint가 없어 트래픽이 전달되지 않습니다.

## 확인

```bash
ns=<namespace>
svc=<service>
kubectl get svc "$svc" -n "$ns" -o wide
kubectl get endpoints "$svc" -n "$ns" -o wide
kubectl get endpointslice -n "$ns" -l kubernetes.io/service-name="$svc"
kubectl describe svc "$svc" -n "$ns"
```

## 판단

- Service selector와 Pod label이 일치하는지 확인합니다.
- Pod가 Ready 상태인지 확인합니다.
- readiness probe 실패로 endpoint에서 제외됐는지 확인합니다.
- targetPort가 컨테이너 port와 맞는지 확인합니다.

## 관련 문서

- `docs/objects/service.md`
- `docs/network/cni-service-proxy.md`
