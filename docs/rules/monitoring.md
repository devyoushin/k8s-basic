# 모니터링 및 확인 기준

Kubernetes 문서의 모니터링/확인 섹션 작성 기준입니다.

---

## 1. 기본 상태 확인 명령어

모든 문서의 모니터링 섹션에 반드시 포함:

```bash
# Pod 상태
kubectl get pods -n <NAMESPACE> -o wide

# 이벤트 확인 (장애 발생 시 최우선)
kubectl get events -n <NAMESPACE> --sort-by='.lastTimestamp'

# 리소스 상세
kubectl describe <RESOURCE_TYPE> <NAME> -n <NAMESPACE>
```

## 2. 리소스별 핵심 확인 항목

| 리소스 | 핵심 확인 명령어 | 주요 상태값 |
|--------|---------------|-----------|
| Pod | `kubectl get pods` | Running, Pending, CrashLoopBackOff |
| Deployment | `kubectl rollout status` | 배포 완료 여부 |
| Node | `kubectl get nodes` | Ready, NotReady |
| PVC | `kubectl get pvc` | Bound, Pending |
| Service | `kubectl get endpoints` | Endpoints 존재 여부 |

## 3. 메트릭 서버 활용

```bash
# Node 리소스 사용량
kubectl top nodes

# Pod 리소스 사용량
kubectl top pods -n <NAMESPACE>
```

## 4. 로그 확인 패턴

```bash
# 현재 로그
kubectl logs <POD> -n <NAMESPACE>

# 이전 컨테이너 로그 (재시작 후)
kubectl logs <POD> -n <NAMESPACE> --previous

# 실시간 스트리밍
kubectl logs -f <POD> -n <NAMESPACE>

# 특정 컨테이너 (멀티컨테이너 Pod)
kubectl logs <POD> -c <CONTAINER> -n <NAMESPACE>
```

## 5. 알람 기준 예시

| 상황 | 조건 | 대응 |
|------|------|------|
| Pod 재시작 | RestartCount > 3 in 1h | 로그/이벤트 확인 |
| Node NotReady | 1개 이상 | 즉시 확인 |
| OOMKilled | 발생 시 | memory limits 상향 |
