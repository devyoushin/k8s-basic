# 보안 검토 체크리스트

Kubernetes 문서 및 YAML 작성 시 반드시 확인해야 할 보안 항목입니다.

---

## 1. Pod 보안

- [ ] `securityContext.runAsNonRoot: true` 설정
- [ ] `capabilities.drop: ["ALL"]` 설정
- [ ] `readOnlyRootFilesystem: true` (쓰기 필요 시 emptyDir 사용)
- [ ] `privileged: false` 명시
- [ ] `hostNetwork`, `hostPID`, `hostIPC`: false

## 2. RBAC 최소 권한

- [ ] `cluster-admin` 사용 시 반드시 주의 경고 추가
- [ ] Role/ClusterRole은 필요한 리소스/동사만 허용
- [ ] ServiceAccount: 용도별 분리
- [ ] `automountServiceAccountToken: false` (API 접근 불필요 시)

## 3. Secret 관리

- [ ] Secret을 YAML에 평문으로 작성 금지
- [ ] 환경변수로 직접 Secret 노출 금지 (secretKeyRef 사용)
- [ ] Secret 예시에 `<BASE64_VALUE>` 플레이스홀더 사용

## 4. 네트워크 정책

- [ ] NetworkPolicy 없이 모든 트래픽 허용하는 설정에 경고
- [ ] `0.0.0.0/0` 사용 시 반드시 주의 문구 추가

## 5. 이미지 보안

- [ ] `image:latest` 태그 사용 금지
- [ ] 프라이빗 레지스트리 사용 시 imagePullSecrets 명시

## 6. 금지 표현

- 실제 클러스터 이름/IP/도메인 노출 금지
- 실제 계정 ID/ARN 노출 금지
- 모든 민감 정보는 `<YOUR_VALUE>` 형식으로 대체
