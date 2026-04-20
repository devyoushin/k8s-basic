# Kubernetes 코드 작성 규칙

이 저장소의 YAML 매니페스트 및 kubectl 코드 작성 규칙입니다.

---

## 1. YAML 매니페스트 규칙

### 기본 원칙
- `apiVersion`: 안정 버전 우선 (GA > beta > alpha)
- `namespace` 항상 명시 (default namespace 사용 금지)
- 레이블 필수: `app`, `version`

### 필수 레이블 패턴
```yaml
labels:
  app: <APP_NAME>        # 애플리케이션 이름
  version: <VERSION>     # 버전 (v1, v2)
  managed-by: kubectl    # 관리 도구
```

### Pod 보안 기본값
```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  readOnlyRootFilesystem: true
  capabilities:
    drop: ["ALL"]
```

### Resources 필수 설정
```yaml
resources:
  requests:
    memory: "128Mi"
    cpu: "100m"
  limits:
    memory: "256Mi"
    cpu: "500m"
```

## 2. kubectl 명령어 규칙

- `-n <NAMESPACE>` 항상 명시
- 조회: `kubectl get`, 상세: `kubectl describe`
- 로그: `kubectl logs --previous` (재시작된 컨테이너)
- 플레이스홀더: `<POD_NAME>`, `<NAMESPACE>` 형식

### 자주 쓰는 진단 패턴
```bash
# Pod 상태 진단
kubectl get pods -n <NAMESPACE> -o wide
kubectl describe pod <POD_NAME> -n <NAMESPACE>
kubectl logs <POD_NAME> -n <NAMESPACE> --previous

# 이벤트 확인
kubectl get events -n <NAMESPACE> --sort-by='.lastTimestamp'
```

## 3. 플레이스홀더 표기법

| 타입 | 형식 |
|------|------|
| 네임스페이스 | `<NAMESPACE>` |
| Pod 이름 | `<POD_NAME>` |
| 컨테이너 이미지 | `<IMAGE>:<TAG>` |
| 클러스터 이름 | `<CLUSTER_NAME>` |
