## 1. 개요 및 비유

소프트웨어 공급망 보안은 컨테이너 이미지가 빌드→서명→배포→실행되는 전 과정의 무결성을 보장합니다.

💡 **비유하자면 의약품의 제조→유통→복용 전 과정 추적 시스템과 같습니다.**
누가 만들었는지(출처), 유통 과정에서 변조되지 않았는지(무결성), 유통기한(만료), 성분표(SBOM)를 모두 검증합니다.

---

## 2. 위협 모델 (공격 시나리오)

```
공급망 공격 포인트:
┌─────────────────────────────────────────────────────────┐
│ 소스코드 → [빌드 시스템] → [이미지 레지스트리] → 클러스터│
│                                                         │
│ ① 소스코드 오염: 악성 의존성 삽입 (SolarWinds 사례)     │
│ ② 빌드 시스템 침해: CI/CD 파이프라인 변조               │
│ ③ 이미지 레지스트리 침해: 푸시된 이미지 교체            │
│ ④ 전송 중 변조: MITM 공격으로 이미지 교체              │
│ ⑤ 배포 시 검증 없음: 미서명 이미지 실행                │
└─────────────────────────────────────────────────────────┘

방어 체계:
① SBOM으로 의존성 추적
② SLSA 프레임워크로 빌드 무결성 보장
③ cosign으로 이미지 서명 및 검증
④ TLS + 다이제스트 고정(digest pinning)
⑤ Admission Controller로 서명 미검증 이미지 차단
```

---

## 3. 이미지 다이제스트 고정 (Digest Pinning)

태그(`:latest`, `:v1.0`)는 변경될 수 있습니다. 다이제스트는 변경 불가능합니다.

```bash
# 현재 태그의 다이제스트 확인
docker inspect nginx:1.25 --format='{{index .RepoDigests 0}}'
# nginx@sha256:a484819eb60211f5299034ac80f6a681b06f89e65866ce91f356ed7c72af059c

# 또는 crane 도구 사용
crane digest nginx:1.25
# sha256:a484819eb60211f5299034ac80f6a681b06f89e65866ce91f356ed7c72af059c
```

```yaml
# 태그 사용 (위험 — 이미지가 교체될 수 있음)
image: nginx:1.25

# 다이제스트 고정 (권장 — 항상 동일한 이미지)
image: nginx@sha256:a484819eb60211f5299034ac80f6a681b06f89e65866ce91f356ed7c72af059c
```

---

## 4. cosign — 이미지 서명 및 검증

cosign은 Sigstore 프로젝트의 컨테이너 이미지 서명 도구입니다.

### 4.1 이미지 서명

```bash
# cosign 키쌍 생성
cosign generate-key-pair
# 생성: cosign.key (프라이빗), cosign.pub (퍼블릭)

# 이미지 서명
cosign sign --key cosign.key \
  myregistry.io/my-app@sha256:abc123...
# → 레지스트리에 서명 데이터를 별도 태그로 저장
# → myregistry.io/my-app:sha256-abc123....sig

# 빌드 메타데이터 어테스테이션 추가
cosign attest --key cosign.key \
  --predicate sbom.json \
  --type spdxjson \
  myregistry.io/my-app@sha256:abc123...

# 서명 검증
cosign verify --key cosign.pub \
  myregistry.io/my-app@sha256:abc123...
# 출력: Verification for myregistry.io/my-app@sha256:abc123 -- SUCCESS
```

### 4.2 Keyless 서명 (OIDC 기반, CI/CD 권장)

키를 관리하지 않고 CI/CD의 OIDC 토큰으로 서명합니다. Sigstore의 Fulcio CA와 Rekor 투명성 로그를 사용합니다.

```bash
# GitHub Actions에서 keyless 서명
# .github/workflows/build.yaml:
# - name: Sign image
#   env:
#     COSIGN_EXPERIMENTAL: 1   # Keyless 모드 활성화
#   run: |
#     cosign sign \
#       --oidc-issuer=https://token.actions.githubusercontent.com \
#       myregistry.io/my-app@${{ steps.build.outputs.digest }}

# Keyless 검증 (서명자 identity 확인)
COSIGN_EXPERIMENTAL=1 cosign verify \
  --certificate-identity="https://github.com/myorg/myrepo/.github/workflows/build.yaml@refs/heads/main" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  myregistry.io/my-app@sha256:abc123...
```

---

## 5. SBOM (Software Bill of Materials)

컨테이너 이미지에 포함된 모든 패키지와 라이브러리 목록입니다.

```bash
# syft로 이미지 SBOM 생성
syft myregistry.io/my-app:1.0 -o spdx-json > sbom.spdx.json
syft myregistry.io/my-app:1.0 -o cyclonedx-json > sbom.cdx.json

# SBOM 내용 확인 (패키지 목록)
cat sbom.spdx.json | jq '.packages[] | {name: .name, version: .versionInfo}'

# grype로 SBOM 취약점 스캔
grype sbom:./sbom.spdx.json
# 또는 이미지 직접 스캔
grype myregistry.io/my-app:1.0

# SBOM을 이미지에 어테스테이션으로 첨부
cosign attest --key cosign.key \
  --predicate sbom.spdx.json \
  --type spdxjson \
  myregistry.io/my-app@sha256:abc123...
```

---

## 6. Policy — 미서명 이미지 배포 차단

### 6.1 Kyverno로 서명 검증 강제

```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: verify-image-signature
spec:
  validationFailureAction: Enforce   # 위반 시 배포 차단
  rules:
  - name: check-image-signature
    match:
      any:
      - resources:
          kinds: [Pod]
    verifyImages:
    - imageReferences:
      - "myregistry.io/my-app*"      # 검증 대상 이미지 패턴
      attestors:
      - entries:
        - keys:
            publicKeys: |-
              -----BEGIN PUBLIC KEY-----
              MFkwEwYHKoZIzj0CAQY...
              -----END PUBLIC KEY-----
        # Keyless 서명인 경우:
        # - keyless:
        #     issuer: https://token.actions.githubusercontent.com
        #     subject: https://github.com/myorg/myrepo/*

      # SBOM 어테스테이션 검증 (선택)
      attestations:
      - predicateType: https://spdx.dev/Document
        conditions:
        - all:
          - key: "{{ packages[].name }}"
            operator: NotIn
            value: ["log4j-core"]   # 특정 취약 패키지 포함 이미지 차단
```

### 6.2 SLSA (Supply chain Levels for Software Artifacts)

```
SLSA 레벨 (보안 성숙도 프레임워크):

Level 1: 빌드 출처(provenance) 문서 존재
  - 어떤 소스에서 빌드됐는지 기록

Level 2: 서명된 출처
  - 빌드 서비스가 출처에 서명

Level 3: 검증 가능한 빌드 서비스
  - 빌드 환경이 격리됨 (Hermetic Build)
  - 외부 영향 없이 재현 가능한 빌드

Level 4: 이중화된 리뷰 및 hermetic
  - 두 명 이상의 코드 리뷰
  - 완전 격리된 빌드 환경

# GitHub Actions SLSA Level 3 예시
# slsa-github-generator 사용
jobs:
  build:
    uses: slsa-framework/slsa-github-generator/.github/workflows/builder_go_slsa3.yml@v1.9.0
```

---

## 7. 전체 파이프라인 예시

```
Git Push
  │
  ▼ GitHub Actions CI
1. 코드 빌드 & 테스트
2. 이미지 빌드
3. Trivy/Grype로 취약점 스캔
   → 심각도 HIGH/CRITICAL 있으면 파이프라인 중단
4. syft로 SBOM 생성
5. cosign으로 keyless 서명
6. SBOM을 이미지에 어테스테이션으로 첨부
7. 레지스트리에 다이제스트로 Push
  │
  ▼ ArgoCD (GitOps)
8. 이미지 다이제스트를 manifest에 업데이트
9. PR 생성 → 승인 → merge
  │
  ▼ 클러스터 배포 시
10. Kyverno Admission Controller
    - cosign 서명 검증
    - SBOM 어테스테이션 확인
    - 금지된 패키지 포함 이미지 차단
```

---

## 8. 트러블슈팅

* **cosign 서명 검증 실패:**
  ```bash
  # 서명이 존재하는지 확인
  cosign triangulate myregistry.io/my-app@sha256:abc123
  # → 서명 저장 위치 출력

  # 상세 검증 과정 확인
  cosign verify --key cosign.pub \
    myregistry.io/my-app@sha256:abc123 \
    --output text 2>&1 | head -30

  # 레지스트리 인증 문제인 경우
  cosign login myregistry.io -u user -p password
  ```

* **Kyverno가 서명 없는 이미지 차단:**
  ```bash
  # 어떤 정책이 차단했는지 확인
  kubectl describe pod my-pod | grep -A5 "Events:"
  # "blocked by policy: verify-image-signature"

  # 정책 일시 비활성화 (긴급 시)
  kubectl annotate clusterpolicy verify-image-signature \
    policies.kyverno.io/scored=false

  # 또는 특정 네임스페이스 제외
  kubectl label namespace testing kyverno.io/exclude=true
  ```
