# DNS Debug Lab

CoreDNS와 Service DNS resolution을 확인합니다.

## 확인

```bash
kubectl get pods -n kube-system -l k8s-app=kube-dns -o wide
kubectl get svc -n kube-system kube-dns
bash ops/scripts/dns-check.sh default kubernetes.default.svc.cluster.local
```

## 디버그 Pod

```bash
kubectl run netshoot --image nicolaka/netshoot --restart Never -it --rm -- bash
dig kubernetes.default.svc.cluster.local
nslookup kubernetes.default.svc.cluster.local
```

## 확인 포인트

- Pod의 `/etc/resolv.conf` search domain을 확인할 수 있는가?
- Service 이름과 FQDN의 차이를 설명할 수 있는가?
- CoreDNS Pod 로그를 확인할 수 있는가?
