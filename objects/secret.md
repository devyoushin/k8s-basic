## 1. 개요 및 비유
**Secret(시크릿)**은 비밀번호, API 키, TLS 인증서, OAuth 토큰 등 민감한 데이터를 저장하는 쿠버네티스 오브젝트입니다. ConfigMap과 구조는 같지만 데이터가 Base64 인코딩되어 저장되고, RBAC로 접근을 제한할 수 있습니다.

💡 **비유하자면 '자물쇠가 달린 직원 사물함'과 같습니다.**
회사 공용 게시판(ConfigMap)에는 회의실 예약 규칙 같은 공개 정보를 붙이지만, 개인 통장 비밀번호(Secret)는 자물쇠 달린 사물함에 넣어 두고 열쇠를 가진 사람만 꺼낼 수 있게 합니다.

## 2. 핵심 설명
* **Base64 인코딩 ≠ 암호화:** Secret의 값은 Base64로 인코딩되어 있어 보이지 않지만, 누구나 디코딩할 수 있습니다. 진짜 보안을 위해서는 etcd 암호화(`Encryption at Rest`) 또는 **AWS Secrets Manager / Vault**와 같은 외부 시크릿 관리 시스템을 연동해야 합니다.
* **Secret 타입:** 용도에 따라 타입이 다릅니다.
  * `Opaque` (기본): 일반 키-값 데이터
  * `kubernetes.io/tls`: TLS 인증서/키 쌍
  * `kubernetes.io/dockerconfigjson`: 프라이빗 레지스트리 인증 정보
  * `kubernetes.io/service-account-token`: ServiceAccount 토큰
* **주입 방식:** ConfigMap과 동일하게 환경 변수 또는 볼륨 마운트로 주입합니다. 볼륨 마운트 방식이 더 안전합니다(env는 프로세스 목록에서 보일 수 있음).

## 3. YAML 적용 예시

### Secret 생성 (명령어 - 권장)
```bash
# 직접 값 지정 (Base64 인코딩 자동)
kubectl create secret generic db-credentials \
  --from-literal=username=admin \
  --from-literal=password='S!B\*d$zDsb='

# 파일에서 생성
kubectl create secret generic tls-secret \
  --from-file=tls.crt=server.crt \
  --from-file=tls.key=server.key
```

### Secret YAML (Base64 값 직접 입력)
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: db-credentials
  namespace: default
type: Opaque
data:
  username: YWRtaW4=      # echo -n 'admin' | base64
  password: UEBzc3dvcmQx  # echo -n 'P@ssword1' | base64
```

### 파드에서 Secret 사용 (볼륨 마운트 방식 - 권장)
```yaml
spec:
  containers:
  - name: app
    image: my-app:1.0
    volumeMounts:
    - name: secret-vol
      mountPath: /etc/secrets  # 각 키가 파일로 마운트됨
      readOnly: true
  volumes:
  - name: secret-vol
    secret:
      secretName: db-credentials
```

### 파드에서 Secret 사용 (환경 변수 방식)
```yaml
spec:
  containers:
  - name: app
    env:
    - name: DB_PASSWORD
      valueFrom:
        secretKeyRef:
          name: db-credentials
          key: password
```

## 4. 트러블 슈팅
* **Secret을 수정했는데 파드에 반영이 안 됨:**
  * 환경 변수로 주입한 경우 파드 재시작이 필요합니다. 볼륨 마운트로 주입한 경우 kubelet이 주기적으로 동기화하지만(약 1분), 앱이 파일을 동적으로 다시 읽는 기능이 있어야 합니다.
* **`Error from server (Forbidden): secrets is forbidden`:**
  * 현재 ServiceAccount에 Secret 읽기 권한이 없는 것입니다. RBAC에서 `Role`/`ClusterRole`에 `secrets` 리소스에 대한 `get`, `list` 권한을 부여하세요.
* **프라이빗 레지스트리에서 이미지를 못 가져올 때:**
  * `imagePullSecrets`를 파드 스펙에 추가하거나, 네임스페이스의 `default` ServiceAccount에 imagePullSecrets를 자동 주입하도록 설정하세요.
