# Rollout Failed Runbook

## 증상

Deployment rollout이 완료되지 않거나 새 ReplicaSet이 정상화되지 않습니다.

## 확인

```bash
ns=<namespace>
deploy=<deployment>
kubectl rollout status deployment/"$deploy" -n "$ns"
kubectl rollout history deployment/"$deploy" -n "$ns"
kubectl describe deployment "$deploy" -n "$ns"
kubectl get rs,pod -n "$ns" -l app="$deploy" -o wide
```

## 판단

- 새 Pod가 Pending, ImagePullBackOff, CrashLoopBackOff인지 확인합니다.
- maxUnavailable/maxSurge 설정을 확인합니다.
- readiness probe 실패로 rollout이 멈췄는지 확인합니다.
- 필요하면 이전 revision으로 rollback합니다.

## 롤백

```bash
kubectl rollout undo deployment/"$deploy" -n "$ns"
```

## 관련 문서

- `docs/deep-dive/deployment-strategy.md`
- `docs/objects/deployment.md`
