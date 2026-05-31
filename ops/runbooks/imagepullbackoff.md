# ImagePullBackOff Runbook

## 증상

Pod가 이미지를 가져오지 못해 `ImagePullBackOff` 또는 `ErrImagePull` 상태가 됩니다.

## 확인

```bash
ns=<namespace>
pod=<pod>
kubectl describe pod "$pod" -n "$ns"
kubectl get events -n "$ns" --sort-by=.lastTimestamp | tail -30
```

## 판단

- image 이름과 tag가 정확한지 확인합니다.
- private registry라면 imagePullSecret 연결을 확인합니다.
- node에서 registry 접근이 가능한지 확인합니다.
- rate limit 또는 인증 실패 메시지를 확인합니다.

## 관련 문서

- `docs/security/image-security.md`
- `docs/deep-dive/container-runtime.md`
