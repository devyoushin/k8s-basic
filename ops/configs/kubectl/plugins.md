# kubectl Plugin Notes

운영과 학습에 유용한 kubectl 플러그인 후보입니다.

## krew

```bash
kubectl krew version
kubectl krew search
```

## 후보

```bash
kubectl krew install ctx
kubectl krew install ns
kubectl krew install neat
kubectl krew install tree
kubectl krew install resource-capacity
```

## 용도

- `ctx`: context 전환
- `ns`: namespace 전환
- `neat`: 불필요한 managedFields 제거
- `tree`: ownerReference 기반 리소스 관계 확인
- `resource-capacity`: node별 resource capacity 확인
