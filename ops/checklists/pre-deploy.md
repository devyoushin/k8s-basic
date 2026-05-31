# Pre Deploy Checklist

## Manifest

- [ ] selector와 label이 일치한다.
- [ ] resource requests/limits가 설정되어 있다.
- [ ] readinessProbe가 있다.
- [ ] Secret 값이 manifest에 평문으로 들어가지 않았다.
- [ ] namespace가 명시되어 있거나 배포 context가 명확하다.

## Rollout

- [ ] rollout strategy와 replica 수를 확인했다.
- [ ] PDB가 필요한 워크로드에 설정되어 있다.
- [ ] rollback 명령을 준비했다.
- [ ] 배포 후 확인할 로그와 metric을 정했다.

## 빠른 명령

```bash
kubectl diff -f <manifest>
kubectl apply --dry-run=server -f <manifest>
kubectl rollout status deployment/<name> -n <namespace>
```
