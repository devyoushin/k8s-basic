## 1. 개요 및 비유
**컨테이너 이미지 보안**은 컨테이너를 실행하기 전 단계, 즉 이미지 빌드와 배포 파이프라인에서의 보안을 다룹니다. 취약한 이미지를 클러스터에 배포하는 것은 구멍 난 금고를 설치하는 것과 같습니다.

💡 **비유하자면 '식품 위생 검사'와 같습니다.**
식당(클러스터)에 식재료(이미지)를 들여오기 전에 유통기한(취약점)을 확인하고, 원산지 증명서(서명)를 검사하는 것과 같습니다. 아무리 조리 환경이 깨끗해도 상한 재료를 쓰면 문제가 생깁니다.

## 2. 핵심 설명

### 이미지 보안의 4가지 레이어

```
1. 베이스 이미지 선택      → 최소한의 이미지 사용 (distroless, alpine)
2. 빌드 시 취약점 스캔     → Trivy, Grype, Snyk
3. 이미지 서명 & 검증      → Cosign, Notation
4. 런타임 정책 강제         → OPA Gatekeeper, Kyverno로 미서명/취약 이미지 차단
```

### 베이스 이미지 비교

| 이미지 | 크기 | 쉘 | 취약점 수 | 용도 |
|---|---|---|---|---|
| `ubuntu:22.04` | ~70MB | ✓ | 많음 | 개발/디버깅 |
| `alpine:3.19` | ~7MB | ✓ (ash) | 적음 | 범용 소형 |
| `distroless/java21` | ~200MB | ✗ | 매우 적음 | 프로덕션 Java |
| `distroless/static` | ~2MB | ✗ | 거의 없음 | Go static binary |
| `scratch` | 0MB | ✗ | 없음 | Go/Rust 완전 정적 빌드 |

**distroless 이미지란?** Google이 관리하는 이미지로, 쉘, 패키지 매니저 등 앱 실행에 불필요한 요소를 제거한 이미지입니다. 공격자가 컨테이너에 들어와도 쉘이 없어서 할 수 있는 것이 극히 제한됩니다.

### 멀티스테이지 빌드 + distroless 조합

```dockerfile
# 빌드 스테이지: 컴파일러, 빌드 도구 포함 (최종 이미지에 포함 안 됨)
FROM golang:1.22 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# 실행 스테이지: 바이너리만 복사, 쉘 없는 distroless 사용
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /app/server /server
USER nonroot:nonroot   # nonroot UID로 실행
ENTRYPOINT ["/server"]
```

## 3. 실습 예시

### Trivy로 이미지 취약점 스캔
```bash
# 이미지 스캔 (HIGH, CRITICAL 취약점만 표시)
trivy image --severity HIGH,CRITICAL nginx:1.25

# 파일시스템 스캔 (CI 파이프라인에서)
trivy fs --severity HIGH,CRITICAL .

# SBOM(소프트웨어 자재 명세서) 생성
trivy image --format cyclonedx --output sbom.json nginx:1.25

# Kubernetes 클러스터 내 실행 중인 이미지 스캔
trivy k8s --report summary cluster
```

### Cosign으로 이미지 서명 & 검증
```bash
# 키 생성
cosign generate-key-pair

# 이미지 서명 (빌드 후 레지스트리 푸시 직후)
cosign sign --key cosign.key my-registry/my-app:1.0

# 서명 검증
cosign verify --key cosign.pub my-registry/my-app:1.0

# keyless 서명 (OIDC + Sigstore Rekor 투명성 로그 사용)
cosign sign my-registry/my-app:1.0  # GitHub Actions OIDC 토큰 자동 사용
```

### Kyverno로 서명된 이미지만 배포 허용
```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: verify-image-signature
spec:
  validationFailureAction: Enforce  # 위반 시 배포 차단
  rules:
  - name: check-image-signature
    match:
      any:
      - resources:
          kinds: [Pod]
    verifyImages:
    - imageReferences:
      - "my-registry/*"             # 이 레지스트리의 모든 이미지 검증
      attestors:
      - entries:
        - keys:
            publicKeys: |-          # cosign 공개키
              -----BEGIN PUBLIC KEY-----
              MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE...
              -----END PUBLIC KEY-----
```

### Kyverno로 latest 태그 금지
```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: disallow-latest-tag
spec:
  validationFailureAction: Enforce
  rules:
  - name: require-image-tag
    match:
      any:
      - resources:
          kinds: [Pod]
    validate:
      message: "이미지 태그에 'latest'는 사용할 수 없습니다. 명시적 버전 태그를 사용하세요."
      pattern:
        spec:
          containers:
          - image: "*:*"            # 태그가 반드시 있어야 함
          =(initContainers):
          - image: "*:*"
```

### OPA Gatekeeper로 허용된 레지스트리만 사용
```yaml
# ConstraintTemplate 정의
apiVersion: templates.gatekeeper.sh/v1
kind: ConstraintTemplate
metadata:
  name: allowedregistries
spec:
  crd:
    spec:
      names:
        kind: AllowedRegistries
  targets:
  - target: admission.k8s.gatekeeper.sh
    rego: |
      package allowedregistries
      violation[{"msg": msg}] {
        container := input.review.object.spec.containers[_]
        not startswith(container.image, "my-registry.example.com/")
        not startswith(container.image, "gcr.io/distroless/")
        msg := sprintf("허용되지 않은 레지스트리입니다: %v", [container.image])
      }

---
# Constraint 적용
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: AllowedRegistries
metadata:
  name: prod-allowed-registries
spec:
  match:
    namespaces: ["production"]
```

### CI 파이프라인 통합 예시 (GitHub Actions)
```yaml
# .github/workflows/build-push.yml
jobs:
  build-and-push:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4

    - name: Build image
      run: docker build -t $IMAGE_TAG .

    - name: Scan with Trivy
      uses: aquasecurity/trivy-action@master
      with:
        image-ref: ${{ env.IMAGE_TAG }}
        severity: 'CRITICAL'
        exit-code: '1'          # CRITICAL 취약점 있으면 파이프라인 실패

    - name: Push image
      run: docker push $IMAGE_TAG

    - name: Sign image with Cosign
      uses: sigstore/cosign-installer@v3
      run: cosign sign --yes $IMAGE_TAG  # keyless signing
```

## 4. 트러블 슈팅

* **distroless 컨테이너 디버깅 방법 (쉘이 없음):**
  ```bash
  # 방법 1: 같은 파드에 디버그 컨테이너 임시 추가 (k8s 1.23+)
  kubectl debug -it <파드명> --image=busybox --target=<컨테이너명>

  # 방법 2: 노드에서 직접 접근
  kubectl debug node/<노드명> -it --image=ubuntu
  ```

* **Trivy 스캔에서 false positive (오탐)가 너무 많을 때:**
  * `.trivyignore` 파일로 특정 CVE를 무시할 수 있습니다.
  ```
  # .trivyignore
  CVE-2023-12345  # 우리 앱에서 실제로 사용하지 않는 기능의 취약점
  ```

* **이미지 레이어에 시크릿이 포함된 것을 뒤늦게 발견:**
  * 이미지 히스토리를 삭제해도 레이어는 남아있습니다. 해당 이미지를 즉시 레지스트리에서 삭제하고 노출된 시크릿(API 키 등)을 모두 교체해야 합니다.
  * 빌드 시 시크릿은 반드시 `--secret` 플래그나 BuildKit의 시크릿 마운트 기능을 사용하세요.
  ```dockerfile
  # 시크릿을 레이어에 남기지 않는 올바른 방법
  RUN --mount=type=secret,id=npm_token \
      NPM_TOKEN=$(cat /run/secrets/npm_token) npm install
  ```
