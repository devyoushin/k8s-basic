# DNS Failure Runbook

## 증상

Pod 내부에서 Service DNS 또는 외부 도메인 이름 해석이 실패합니다.

## 확인

```bash
kubectl get pods -n kube-system -l k8s-app=kube-dns -o wide
kubectl logs -n kube-system -l k8s-app=kube-dns --tail=100
bash ops/scripts/dns-check.sh default kubernetes.default.svc.cluster.local
```

## 판단

- CoreDNS Pod가 정상인지 확인합니다.
- kube-dns Service와 endpoint를 확인합니다.
- Pod의 `/etc/resolv.conf`를 확인합니다.
- NodeLocal DNSCache를 사용하는 경우 DaemonSet 상태를 확인합니다.

## 관련 문서

- `docs/deep-dive/dns-service-discovery.md`
- `docs/network/coredns.md`
- `docs/deep-dive/node-local-dns.md`
