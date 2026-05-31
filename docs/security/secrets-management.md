## 1. 개요 및 비유
쿠버네티스 기본 Secret은 etcd에 Base64로 저장되어 보안이 취약합니다. **외부 시크릿 관리 시스템(AWS Secrets Manager, HashiCorp Vault 등)과 연동**하면 시크릿을 외부에서 안전하게 관리하고, 쿠버네티스에는 참조만 두는 방식을 구현할 수 있습니다.

💡 **비유하자면 '열쇠를 직접 갖고 다니는 것 vs 금고회사 앱으로 필요할 때만 여는 것'의 차이입니다.**
기본 Secret은 집 열쇠(비밀번호)를 수첩(etcd)에 적어두는 것과 같습니다. 외부 시크릿 관리는 금고회사(AWS/Vault)가 열쇠를 보관하고, 인증된 직원(ServiceAccount)이 필요할 때만 디지털로 열어주는 방식입니다.

## 2. 핵심 설명

### 기본 Secret의 문제점

| 문제 | 설명 |
|---|---|
| **Base64 = 평문** | Base64는 인코딩이지 암호화가 아님. `echo <값> \| base64 -d` 로 즉시 복호화 가능 |
| **etcd 접근 = 전체 노출** | etcd에 접근 권한이 있으면 모든 Secret이 노출됨 |
| **감사 추적 어려움** | 누가 언제 Secret을 조회했는지 추적하기 어려움 |
| **로테이션 번거로움** | 시크릿 교체 시 파드 재시작이 필요한 경우가 많음 |

### 외부 시크릿 관리 솔루션 비교

| 솔루션 | 방식 | 적합한 환경 |
|---|---|---|
| **External Secrets Operator (ESO)** | 외부 시크릿을 K8s Secret으로 동기화 | AWS/GCP/Azure + K8s 모든 환경 |
| **Secrets Store CSI Driver** | 외부 시크릿을 볼륨으로 직접 마운트 | 환경변수 대신 파일로 주입 원할 때 |
| **Vault Agent Injector** | Vault Agent 사이드카로 자동 주입 | HashiCorp Vault 사용 환경 |

### etcd 암호화 (Encryption at Rest)
외부 솔루션 없이 기본 Secret을 강화하는 방법입니다.

```yaml
# /etc/kubernetes/enc/encryption-config.yaml (API Server에 설정)
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
- resources:
  - secrets
  providers:
  - aescbc:
      keys:
      - name: key1
        secret: <base64로 인코딩된 32바이트 키>
  - identity: {}  # 기존 미암호화 데이터 읽기 위해 fallback으로 유지
```

## 3. YAML 적용 예시

### External Secrets Operator + AWS Secrets Manager
```bash
# ESO 설치
helm install external-secrets \
  external-secrets/external-secrets \
  -n external-secrets-system \
  --create-namespace
```

```yaml
# 1. AWS 인증 설정 (IRSA 방식 - IAM Role for Service Accounts)
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: aws-secretsmanager
  namespace: production
spec:
  provider:
    aws:
      service: SecretsManager
      region: ap-northeast-2
      auth:
        jwt:
          serviceAccountRef:
            name: external-secrets-sa  # IRSA로 AWS 권한이 부여된 SA

---
# 2. 외부 시크릿을 K8s Secret으로 동기화
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: db-credentials
  namespace: production
spec:
  refreshInterval: 1h           # 1시간마다 외부에서 최신값 동기화
  secretStoreRef:
    name: aws-secretsmanager
    kind: SecretStore
  target:
    name: db-credentials        # 생성될 K8s Secret 이름
    creationPolicy: Owner       # ESO가 생성/삭제 관리
  data:
  - secretKey: username         # K8s Secret의 키 이름
    remoteRef:
      key: prod/db/credentials  # AWS Secrets Manager의 시크릿 이름
      property: username        # JSON 내부 필드
  - secretKey: password
    remoteRef:
      key: prod/db/credentials
      property: password
```

### Secrets Store CSI Driver + AWS Secrets Manager (볼륨 마운트 방식)
```yaml
# SecretProviderClass 정의
apiVersion: secrets-store.csi.x-k8s.io/v1
kind: SecretProviderClass
metadata:
  name: aws-secrets
  namespace: production
spec:
  provider: aws
  parameters:
    objects: |
      - objectName: "prod/db/credentials"
        objectType: "secretsmanager"
        jmesPath:
          - path: username
            objectAlias: db-username
          - path: password
            objectAlias: db-password
  # 동시에 K8s Secret도 생성 (환경변수 주입에 필요)
  secretObjects:
  - secretName: db-credentials
    type: Opaque
    data:
    - objectName: db-username
      key: username
    - objectName: db-password
      key: password

---
# 파드에서 CSI 볼륨으로 시크릿 마운트
spec:
  serviceAccountName: app-sa     # IRSA 권한 필요
  containers:
  - name: app
    image: my-app:1.0
    volumeMounts:
    - name: secrets-store
      mountPath: /mnt/secrets     # /mnt/secrets/db-username 파일로 마운트됨
      readOnly: true
  volumes:
  - name: secrets-store
    csi:
      driver: secrets-store.csi.k8s.io
      readOnly: true
      volumeAttributes:
        secretProviderClass: aws-secrets
```

### HashiCorp Vault + Vault Agent Injector
```yaml
# Vault Agent Injector 어노테이션으로 사이드카 자동 주입
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  template:
    metadata:
      annotations:
        vault.hashicorp.com/agent-inject: "true"
        vault.hashicorp.com/role: "my-app"           # Vault Role
        # Vault 경로에서 시크릿 가져와 파일로 렌더링
        vault.hashicorp.com/agent-inject-secret-db: "secret/data/prod/db"
        vault.hashicorp.com/agent-inject-template-db: |
          {{- with secret "secret/data/prod/db" -}}
          export DB_USERNAME="{{ .Data.data.username }}"
          export DB_PASSWORD="{{ .Data.data.password }}"
          {{- end }}
    spec:
      serviceAccountName: my-app-sa
      containers:
      - name: app
        image: my-app:1.0
        command: ["sh", "-c", "source /vault/secrets/db && ./start.sh"]
```

### 시크릿 자동 로테이션 감지 (ESO 사용 시)
```yaml
# ExternalSecret이 업데이트되면 Deployment 자동 재시작
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: api-keys
spec:
  refreshInterval: 15m    # 15분마다 확인
  target:
    name: api-keys
    template:
      metadata:
        annotations:
          # 시크릿 내용이 바뀌면 이 해시값이 변경됨
          # Reloader 같은 툴이 이를 감지하여 파드 재시작
          secret-hash: "{{ .apiKey | sha256sum }}"
```

## 4. 트러블 슈팅

* **ExternalSecret이 `SecretSyncedError` 상태:**
  ```bash
  kubectl describe externalsecret db-credentials -n production
  # Events 섹션에서 에러 원인 확인 (권한 부족, 경로 오류 등)
  ```
  * IAM Role에 `secretsmanager:GetSecretValue` 권한이 있는지, IRSA 설정이 올바른지 확인하세요.

* **시크릿을 업데이트했는데 파드에 반영이 안 됨:**
  * ESO는 K8s Secret을 자동 업데이트하지만, 환경변수로 주입된 값은 파드 재시작 전까지 바뀌지 않습니다.
  * `Reloader` 툴을 사용하면 Secret/ConfigMap 변경 시 파드를 자동 재시작합니다.
  ```bash
  # Reloader 설치 후 Deployment에 어노테이션 추가
  kubectl annotate deployment my-app \
    reloader.stakater.com/auto="true"
  ```

* **CSI 드라이버 볼륨 마운트 실패 (`FailedMount`):**
  * `kubectl describe pod <파드명>` 에서 `MountVolume.SetUp failed` 메시지 확인
  * 주로 ServiceAccount의 IRSA 권한 문제이거나, SecretProviderClass의 경로 오류입니다.
  * `kubectl logs -n kube-system -l app=secrets-store-csi-driver` 로 CSI 드라이버 로그를 확인하세요.

* **etcd 암호화 적용 후 기존 시크릿이 안 읽힘:**
  * `identity: {}` provider를 fallback으로 유지하면 기존 미암호화 데이터도 읽힙니다.
  * 모든 기존 시크릿을 새로 암호화하려면 `kubectl get secrets --all-namespaces -o json | kubectl replace -f -` 로 강제 재저장하세요.
