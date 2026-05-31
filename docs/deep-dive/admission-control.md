## 1. 개요 및 비유
**Admission Control(어드미션 컨트롤)**은 API Server가 요청을 etcd에 저장하기 전 마지막 검문소입니다. 인증/인가를 통과한 요청도 여기서 내용을 수정하거나 거부할 수 있습니다. Kyverno, OPA Gatekeeper 같은 정책 엔진이 이 단계에서 동작합니다.

💡 **비유하자면 '공항 탑승 전 보안 검색대'와 같습니다.**
티켓(인가)이 있어도 보안 검색대(Admission Controller)에서 위험물(privileged 컨테이너, 허용되지 않은 이미지)이 발견되면 탑승을 거부하고, 승인 도장(라벨 자동 추가)을 찍어주기도 합니다.

## 2. 핵심 설명

### API Server 요청 처리 흐름

```
kubectl apply
      ↓
1. Authentication    : 누구인가? (인증서, ServiceAccount 토큰)
      ↓
2. Authorization     : 권한이 있는가? (RBAC)
      ↓
3. Mutating Webhook  : 요청 내용 수정 (라벨 추가, 사이드카 주입 등)
      ↓
4. Object Validation : 스키마 검증 (필수 필드 누락 등)
      ↓
5. Validating Webhook: 정책 위반 여부 확인 (거부 또는 허용)
      ↓
6. etcd 저장
```

### 내장 Admission Controller vs 외부 Webhook

| 구분 | 예시 | 특징 |
|---|---|---|
| **내장 컨트롤러** | LimitRanger, ResourceQuota, PSA | API Server 내부 동작, 별도 설치 불필요 |
| **Mutating Webhook** | Istio 사이드카 주입, ESO | 요청 **수정** 가능, 배포 필요 |
| **Validating Webhook** | Kyverno, OPA Gatekeeper | 요청 **검증** 후 허용/거부, 배포 필요 |

### Kyverno vs OPA Gatekeeper 비교

| 항목 | Kyverno | OPA Gatekeeper |
|---|---|---|
| 정책 언어 | YAML (K8s 친화적) | Rego (전용 언어) |
| 학습 곡선 | 낮음 | 높음 |
| 기능 범위 | 정책 + 이미지 검증 + 뮤테이션 | 정책 검증 중심 |
| 생태계 | Kyverno 정책 라이브러리 풍부 | OPA 에코시스템 |
| 적합한 환경 | K8s 전용, 빠른 도입 | 멀티 플랫폼 정책 통합 |

## 3. YAML 적용 예시

### Kyverno — 정책 패턴 모음

#### 필수 라벨 강제
```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: require-labels
spec:
  validationFailureAction: Enforce   # Audit: 기록만, Enforce: 거부
  rules:
  - name: check-required-labels
    match:
      any:
      - resources:
          kinds: [Deployment]
    validate:
      message: "Deployment에는 'app'과 'team' 라벨이 필수입니다."
      pattern:
        metadata:
          labels:
            app: "?*"    # 비어있지 않은 값이어야 함
            team: "?*"
```

#### 자동으로 라벨/어노테이션 추가 (Mutate)
```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: add-default-labels
spec:
  rules:
  - name: add-team-label
    match:
      any:
      - resources:
          kinds: [Pod]
    mutate:
      patchStrategicMerge:
        metadata:
          labels:
            +(managed-by): kyverno   # 라벨이 없을 때만 추가 (+prefix)
          annotations:
            +(created-at: "{{request.object.metadata.creationTimestamp}}")
```

#### 특권 컨테이너 차단
```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: disallow-privileged
spec:
  validationFailureAction: Enforce
  rules:
  - name: check-privileged
    match:
      any:
      - resources:
          kinds: [Pod]
    validate:
      message: "privileged 컨테이너는 허용되지 않습니다."
      pattern:
        spec:
          containers:
          - =(securityContext):
              =(privileged): "false"   # privileged가 있다면 반드시 false
```

#### 허용된 레지스트리만 사용
```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: allowed-registries
spec:
  validationFailureAction: Enforce
  rules:
  - name: check-registry
    match:
      any:
      - resources:
          kinds: [Pod]
          namespaces: [production, staging]
    validate:
      message: "허용된 레지스트리(my-registry.example.com)의 이미지만 사용할 수 있습니다."
      pattern:
        spec:
          containers:
          - image: "my-registry.example.com/*"
```

#### 리소스 requests 필수 설정
```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: require-requests-limits
spec:
  validationFailureAction: Enforce
  rules:
  - name: check-resources
    match:
      any:
      - resources:
          kinds: [Pod]
    validate:
      message: "모든 컨테이너에 resources.requests와 resources.limits를 설정해야 합니다."
      pattern:
        spec:
          containers:
          - resources:
              requests:
                cpu: "?*"
                memory: "?*"
              limits:
                cpu: "?*"
                memory: "?*"
```

### OPA Gatekeeper — Rego 정책 예시

#### ConstraintTemplate 정의 (허용 레지스트리)
```yaml
apiVersion: templates.gatekeeper.sh/v1
kind: ConstraintTemplate
metadata:
  name: allowedregistries
spec:
  crd:
    spec:
      names:
        kind: AllowedRegistries
      validation:
        openAPIV3Schema:
          properties:
            registries:
              type: array
              items:
                type: string
  targets:
  - target: admission.k8s.gatekeeper.sh
    rego: |
      package allowedregistries

      violation[{"msg": msg}] {
        container := input.review.object.spec.containers[_]
        not registry_allowed(container.image)
        msg := sprintf("이미지 '%v'는 허용된 레지스트리를 사용하지 않습니다.", [container.image])
      }

      registry_allowed(image) {
        registry := input.parameters.registries[_]
        startswith(image, registry)
      }

---
# Constraint 인스턴스 (실제 정책 적용)
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: AllowedRegistries
metadata:
  name: prod-registries
spec:
  match:
    namespaces: [production]
  parameters:
    registries:
    - "my-registry.example.com/"
    - "gcr.io/distroless/"
```

### 커스텀 Mutating Webhook (사이드카 자동 주입 패턴)
```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: my-mutating-webhook
webhooks:
- name: inject-sidecar.example.com
  admissionReviewVersions: ["v1"]
  sideEffects: None
  clientConfig:
    service:
      name: webhook-service      # Webhook 서버 Service
      namespace: webhook-system
      path: /mutate
    caBundle: <base64-CA-cert>   # TLS 인증서
  rules:
  - apiGroups: [""]
    apiVersions: ["v1"]
    resources: ["pods"]
    operations: ["CREATE"]
  namespaceSelector:
    matchLabels:
      inject-sidecar: "true"     # 이 라벨의 네임스페이스에만 적용
  failurePolicy: Ignore          # Webhook 서버 장애 시 요청 허용 (Fail: 거부)
```

```bash
# 정책 위반 현황 확인 (Kyverno)
kubectl get policyreport -A

# 특정 리소스가 정책을 통과하는지 테스트 (dry-run)
kubectl apply -f suspicious-pod.yaml --dry-run=server

# Gatekeeper 위반 목록
kubectl get constraint -A
kubectl describe allowedregistries prod-registries
```

## 4. 트러블 슈팅

* **Webhook 장애로 클러스터 전체 배포가 막힘:**
  * Webhook 서버가 죽으면 `failurePolicy: Fail` 설정 시 모든 파드 생성이 차단됩니다.
  * 긴급 복구 시 Webhook 설정을 임시 삭제하세요.
  ```bash
  kubectl delete mutatingwebhookconfiguration <이름>
  kubectl delete validatingwebhookconfiguration <이름>
  ```
  * Webhook 서버 자체에 반드시 `replicas: 2` 이상과 PDB를 설정하세요.

* **Kyverno 정책이 적용되지 않음:**
  ```bash
  # 정책 상태 확인
  kubectl describe clusterpolicy <정책명>
  # Status.Conditions에서 Ready: True 확인

  # 특정 파드에 정책이 적용됐는지 확인
  kubectl get policyreport -n <네임스페이스> -o yaml
  ```

* **Webhook 응답이 느려서 배포 지연:**
  * `timeoutSeconds`를 낮추고 (기본 10초), Webhook 서버의 응답 시간을 최적화하세요.
  * `namespaceSelector`나 `objectSelector`로 불필요한 리소스에는 Webhook이 호출되지 않도록 범위를 좁히세요.

* **Audit 모드에서 위반 로그 확인:**
  ```bash
  # API Server 감사 로그에서 admission 관련 이벤트 확인
  # (감사 로그 설정이 되어 있는 경우)
  cat /var/log/kubernetes/audit.log | jq 'select(.annotations."admission.k8s.io/mutated-by")'
  ```
