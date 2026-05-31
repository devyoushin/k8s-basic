# Pod OOMKilled Runbook

## 증상

컨테이너가 memory limit을 초과해 `OOMKilled`로 종료됩니다.

## 확인

```bash
ns=<namespace>
pod=<pod>
kubectl describe pod "$pod" -n "$ns"
kubectl top pod "$pod" -n "$ns" --containers
kubectl logs "$pod" -n "$ns" --previous --tail=100
```

## 판단

- limit이 너무 낮은지 확인합니다.
- memory leak인지, 시작 시 peak memory인지 구분합니다.
- VPA 또는 실제 사용량 기반 request/limit 조정을 검토합니다.
- node memory pressure와 eviction 이벤트를 같이 확인합니다.

## 관련 문서

- `docs/deep-dive/oom-eviction.md`
- `docs/deep-dive/resource-management.md`
