## 1. 개요 및 비유

CRD(Custom Resource Definition)와 Webhook을 결합하면 Kubernetes API를 직접 확장할 수 있습니다. 이것이 Operator 패턴의 핵심입니다.

💡 **비유하자면 Kubernetes에 새로운 언어 문법을 추가하는 것과 같습니다.**
CRD로 새 단어(리소스 타입)를 만들고, Webhook으로 문법 검사기(Validating)와 자동 완성(Mutating)을 붙이고, 컨트롤러로 의미를 실제 동작으로 구현합니다.

---

## 2. CRD 설계 심화

### 2.1 OpenAPI v3 스키마 정의

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: databases.mycompany.io
spec:
  group: mycompany.io
  names:
    kind: Database
    listKind: DatabaseList
    plural: databases
    singular: database
    shortNames: [db]              # kubectl get db 가능
    categories: [all]             # kubectl get all 에 포함
  scope: Namespaced
  versions:
  - name: v1
    served: true
    storage: true                  # etcd에 저장할 버전 (하나만)
    subresources:
      status: {}                   # status 서브리소스 활성화
      scale:                       # HPA 연동용 scale 서브리소스
        specReplicasPath: .spec.replicas
        statusReplicasPath: .status.replicas
    additionalPrinterColumns:      # kubectl get 출력 컬럼 추가
    - name: Engine
      type: string
      jsonPath: .spec.engine
    - name: Status
      type: string
      jsonPath: .status.phase
    - name: Age
      type: date
      jsonPath: .metadata.creationTimestamp
    schema:
      openAPIV3Schema:
        type: object
        required: [spec]
        properties:
          spec:
            type: object
            required: [engine, version]
            properties:
              engine:
                type: string
                enum: [postgres, mysql, redis]  # 허용값 제한
                description: 데이터베이스 엔진 종류
              version:
                type: string
                pattern: '^[0-9]+\.[0-9]+$'    # 버전 형식 강제 (정규식)
              replicas:
                type: integer
                minimum: 1
                maximum: 10
                default: 1
              storage:
                type: object
                required: [size]
                properties:
                  size:
                    type: string
                    pattern: '^[0-9]+(Gi|Ti)$'  # "10Gi" 형식
                  storageClass:
                    type: string
          status:
            type: object
            x-kubernetes-preserve-unknown-fields: true  # status는 자유 형식
```

### 2.2 CRD 버전 관리 및 Conversion

여러 버전을 동시에 서빙하려면 Conversion Webhook이 필요합니다.

```yaml
spec:
  versions:
  - name: v1alpha1
    served: true
    storage: false         # 구 버전: 서빙은 하지만 저장은 안 함
  - name: v1
    served: true
    storage: true          # 신 버전: 저장 버전

  conversion:
    strategy: Webhook
    webhook:
      conversionReviewVersions: [v1]
      clientConfig:
        service:
          name: database-webhook
          namespace: my-operator
          path: /convert
        caBundle: <base64-CA-cert>
```

```go
// Conversion Webhook 핸들러 예시
func convertDatabase(req *apiextensionsv1.ConversionRequest) *apiextensionsv1.ConversionResponse {
    for _, obj := range req.Objects {
        cr := &unstructured.Unstructured{}
        cr.UnmarshalJSON(obj.Raw)

        switch req.DesiredAPIVersion {
        case "mycompany.io/v1":
            // v1alpha1 → v1 변환
            engine, _ := unstructured.NestedString(cr.Object, "spec", "dbType")
            unstructured.SetNestedField(cr.Object, engine, "spec", "engine")
            unstructured.RemoveNestedField(cr.Object, "spec", "dbType")
            cr.SetAPIVersion("mycompany.io/v1")
        }
    }
    // ...
}
```

---

## 3. Validating Admission Webhook

배포 전 정책 검사를 수행합니다. 실패 시 배포 거부됩니다.

### 3.1 Webhook 서버 구현 (Go)

```go
package main

import (
    admissionv1 "k8s.io/api/admission/v1"
    "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-mycompany-io-v1-database,
// mutating=false,failurePolicy=fail,
// groups=mycompany.io,resources=databases,verbs=create;update,
// versions=v1,name=vdatabase.kb.io

type DatabaseValidator struct {
    decoder *admission.Decoder
}

func (v *DatabaseValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
    db := &myv1.Database{}
    if err := v.decoder.Decode(req, db); err != nil {
        return admission.Errored(http.StatusBadRequest, err)
    }

    // 검증 로직
    if db.Spec.Engine == "redis" && db.Spec.Replicas > 1 {
        return admission.Denied("Redis는 단일 인스턴스만 지원합니다")
    }

    // 버전 호환성 체크
    if db.Spec.Engine == "postgres" && db.Spec.Version == "9.6" {
        return admission.Denied("PostgreSQL 9.6은 EOL입니다. 14 이상 사용하세요")
    }

    // 경고 (거부하지 않고 경고만)
    warnings := []string{}
    if db.Spec.Replicas == 1 {
        warnings = append(warnings, "단일 복제본은 HA를 보장하지 않습니다")
    }

    return admission.Allowed("").WithWarnings(warnings...)
}
```

### 3.2 Webhook 등록

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: database-validator
  annotations:
    cert-manager.io/inject-ca-from: my-operator/database-webhook-cert
webhooks:
- name: vdatabase.mycompany.io
  admissionReviewVersions: [v1]
  clientConfig:
    service:
      name: database-webhook-service
      namespace: my-operator
      path: /validate-mycompany-io-v1-database
  rules:
  - apiGroups: [mycompany.io]
    apiVersions: [v1]
    resources: [databases]
    operations: [CREATE, UPDATE]
  failurePolicy: Fail        # Webhook 서버 장애 시 배포 거부
  sideEffects: None
  namespaceSelector:
    matchExpressions:
    - key: kubernetes.io/metadata.name
      operator: NotIn
      values: [kube-system]  # 시스템 네임스페이스 제외
```

---

## 4. Mutating Admission Webhook

배포 전 리소스를 자동 수정합니다.

```go
// +kubebuilder:webhook:path=/mutate-mycompany-io-v1-database,
// mutating=true,failurePolicy=fail,...

type DatabaseMutator struct {
    decoder *admission.Decoder
}

func (m *DatabaseMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
    db := &myv1.Database{}
    m.decoder.Decode(req, db)

    original := db.DeepCopy()
    modified := false

    // 기본값 자동 설정
    if db.Spec.Replicas == 0 {
        db.Spec.Replicas = 1
        modified = true
    }

    // 버전 자동 보정 (MySQL이면 최소 8.0)
    if db.Spec.Engine == "mysql" && db.Spec.Version < "8.0" {
        db.Spec.Version = "8.0"
        modified = true
    }

    // 레이블 자동 추가
    if db.Labels == nil {
        db.Labels = map[string]string{}
    }
    db.Labels["managed-by"] = "database-operator"

    if !modified {
        return admission.Allowed("no changes")
    }

    // JSON Patch 생성
    patch, _ := json.Marshal(db)
    return admission.PatchResponseFromRaw(original_raw, patch)
}
```

---

## 5. kubebuilder로 Operator 프로젝트 생성

```bash
# kubebuilder 설치
curl -L https://go.kubebuilder.io/dl/latest/linux/amd64 | tar -xz
sudo mv kubebuilder /usr/local/bin/

# 프로젝트 초기화
mkdir my-operator && cd my-operator
kubebuilder init --domain mycompany.io --repo github.com/mycompany/my-operator

# API 생성 (CRD + Controller + Webhook 스캐폴딩)
kubebuilder create api \
  --group mycompany \
  --version v1 \
  --kind Database \
  --resource=true \
  --controller=true

# Webhook 생성
kubebuilder create webhook \
  --group mycompany \
  --version v1 \
  --kind Database \
  --defaulting \    # MutatingWebhook
  --programmatic-validation  # ValidatingWebhook

# CRD 생성 (types에서 마커 어노테이션 기반)
make manifests

# 로컬 테스트 (envtest 사용)
make test

# 빌드 및 배포
make docker-build docker-push IMG=myregistry/my-operator:v0.1.0
make deploy IMG=myregistry/my-operator:v0.1.0
```

---

## 6. 트러블슈팅

* **Webhook 서버 인증서 오류:**
  ```bash
  # cert-manager가 인증서를 발급했는지 확인
  kubectl get certificate -n my-operator
  kubectl describe certificate database-webhook-cert

  # Secret에 인증서가 있는지
  kubectl get secret -n my-operator | grep webhook

  # Webhook 설정의 caBundle 확인
  kubectl get validatingwebhookconfiguration database-validator \
    -o jsonpath='{.webhooks[0].clientConfig.caBundle}' | base64 -d | openssl x509 -noout -dates
  ```

* **CRD 업데이트 후 기존 리소스 조회 실패:**
  ```bash
  # conversion이 설정됐는지 확인
  kubectl get crd databases.mycompany.io \
    -o jsonpath='{.spec.conversion.strategy}'

  # 기존 리소스를 새 버전으로 강제 마이그레이션
  kubectl get database -A -o json | \
    kubectl apply -f -

  # storage version 업데이트 확인
  kubectl get crd databases.mycompany.io \
    -o jsonpath='{.status.storedVersions}'
  ```

* **Webhook timeout으로 배포 지연:**
  ```bash
  # Webhook 응답 시간 확인
  kubectl get events | grep "webhook"

  # failurePolicy를 Ignore로 임시 변경 (운영 주의)
  kubectl patch validatingwebhookconfiguration database-validator \
    --type='json' \
    -p='[{"op":"replace","path":"/webhooks/0/failurePolicy","value":"Ignore"}]'

  # timeoutSeconds 증가 (최대 30초)
  kubectl patch validatingwebhookconfiguration database-validator \
    --type='json' \
    -p='[{"op":"replace","path":"/webhooks/0/timeoutSeconds","value":15}]'
  ```
