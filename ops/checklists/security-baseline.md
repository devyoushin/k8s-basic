# Security Baseline Checklist

## RBAC

- [ ] cluster-admin 권한 사용자를 최소화했다.
- [ ] ServiceAccount token 자동 mount 필요 여부를 확인했다.
- [ ] Role/ClusterRole scope가 필요한 최소 권한이다.

## Pod Security

- [ ] privileged container를 사용하지 않는다.
- [ ] `runAsNonRoot`를 검토했다.
- [ ] hostNetwork, hostPID, hostPath 사용을 검토했다.
- [ ] image tag와 registry 신뢰 기준을 확인했다.

## Secrets

- [ ] Secret이 Git에 평문으로 저장되지 않는다.
- [ ] 외부 Secret manager 사용 여부를 검토했다.
- [ ] Secret 접근 RBAC가 제한되어 있다.

## 빠른 명령

```bash
kubectl auth can-i --list
kubectl get pods --all-namespaces -o jsonpath='{range .items[*]}{.metadata.namespace}{"\t"}{.metadata.name}{"\t"}{.spec.securityContext}{"\n"}{end}'
```
