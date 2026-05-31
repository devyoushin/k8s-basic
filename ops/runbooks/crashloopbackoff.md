# CrashLoopBackOff Runbook

## 증상

Pod가 반복적으로 시작했다가 종료됩니다.

## 확인

```bash
ns=<namespace>
pod=<pod>
kubectl get pod "$pod" -n "$ns" -o wide
kubectl describe pod "$pod" -n "$ns"
kubectl logs "$pod" -n "$ns" --all-containers --tail=100
kubectl logs "$pod" -n "$ns" --all-containers --previous --tail=100
```

## 판단

- exit code가 애플리케이션 오류인지 OOMKilled인지 확인합니다.
- readiness/liveness probe가 너무 공격적인지 확인합니다.
- ConfigMap, Secret, env, volume mount 실패를 확인합니다.
- 최근 rollout 이후 발생했는지 확인합니다.

## 관련 문서

- `docs/deep-dive/pod-lifecycle.md`
- `docs/deep-dive/graceful-shutdown.md`
- `docs/deep-dive/oom-eviction.md`
