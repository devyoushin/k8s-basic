## 1. 개요 및 비유

**API Server 인증**은 클러스터에 접근하는 모든 주체(사람, 서비스, 노드)가 "나는 누구이고, 무엇을 할 수 있는가"를 증명하는 과정입니다.

💡 **비유하자면 공항 출입국 심사대와 같습니다.**
여권 검사(Authentication) → 비자 확인(Authorization) → 보안 검색(Admission) → 탑승(etcd 저장 후 실제 처리)

---

## 2. 인증(Authentication) 심층 흐름

### 2.1 요청이 API Server에 도달하기까지

```
클라이언트 (kubectl / kubelet / 파드 내 앱)
        │
        │  HTTPS (TLS)
        ▼
[API Server: 6443 포트]
        │
   ┌────▼─────────────────────────────────┐
   │  1. TLS Handshake                     │
   │     - 서버 인증서 제시 (클라이언트 신뢰)  │
   │     - mTLS면 클라이언트 인증서도 제시     │
   └────┬─────────────────────────────────┘
        │
   ┌────▼─────────────────────────────────┐
   │  2. Authentication (인증)             │
   │     - 누구인가? 확인                   │
   └────┬─────────────────────────────────┘
        │
   ┌────▼─────────────────────────────────┐
   │  3. Authorization (인가)              │
   │     - 무엇을 할 수 있는가? (RBAC 등)   │
   └────┬─────────────────────────────────┘
        │
   ┌────▼─────────────────────────────────┐
   │  4. Admission Control                 │
   │     - Mutating → Validating Webhook   │
   └────┬─────────────────────────────────┘
        │
   ┌────▼─────────────────────────────────┐
   │  5. etcd 영속 저장                    │
   └──────────────────────────────────────┘
```

### 2.2 인증 방식별 동작 원리

API Server는 인증 플러그인을 **순서대로** 시도하고, 첫 번째로 성공한 결과를 사용합니다.

#### ① X.509 클라이언트 인증서 (가장 일반적 — kubelet, 노드)

```
kubelet → API Server 인증 과정
┌──────────────────────────────────────────────────┐
│ 1. kubelet이 TLS 핸드셰이크 시 클라이언트 인증서 제시  │
│    (예: /var/lib/kubelet/pki/kubelet-client.crt)  │
│                                                    │
│ 2. API Server가 CA 인증서로 서명 검증               │
│    (--client-ca-file=/etc/kubernetes/pki/ca.crt)  │
│                                                    │
│ 3. 인증서의 CN(Common Name) → 사용자 이름으로 사용   │
│    인증서의 O(Organization) → 그룹으로 사용          │
│                                                    │
│    예: CN=system:node:worker-1                     │
│        O=system:nodes                              │
│    → 사용자: system:node:worker-1                  │
│    → 그룹: system:nodes                            │
└──────────────────────────────────────────────────┘
```

```bash
# 현재 노드의 kubelet 인증서 정보 확인
openssl x509 -in /var/lib/kubelet/pki/kubelet-client-current.pem -noout -subject
# 출력 예: subject=O=system:nodes, CN=system:node:worker-1

# kubeconfig의 인증서 확인
kubectl config view --raw -o jsonpath='{.users[0].user.client-certificate-data}' \
  | base64 -d | openssl x509 -noout -subject
```

#### ② Service Account Token (파드 내부 → API Server 접근)

파드는 자동으로 Service Account 토큰을 마운트 받습니다. Kubernetes 1.21+부터는 **Bound Service Account Token**을 사용합니다.

```
파드 내부 애플리케이션의 인증 흐름
┌──────────────────────────────────────────────────────────┐
│ 1. kubelet이 파드 생성 시 TokenRequest API 호출           │
│    → API Server가 서명된 JWT 토큰 발급 (만료 시간 포함)    │
│                                                          │
│ 2. 토큰을 /var/run/secrets/kubernetes.io/serviceaccount/ │
│    에 projected volume으로 마운트                        │
│                                                          │
│ 3. 앱이 API 호출 시 Authorization: Bearer <token> 헤더   │
│                                                          │
│ 4. API Server의 TokenReview 과정:                        │
│    - JWT 서명 검증 (API Server의 개인키로 서명됨)          │
│    - 토큰의 exp(만료시간) 확인                            │
│    - 토큰에 바인딩된 파드/노드가 여전히 존재하는지 확인      │
│    (Bound Token이므로 파드 삭제 시 토큰도 무효화)          │
└──────────────────────────────────────────────────────────┘
```

```bash
# 파드 내부에서 토큰 확인
cat /var/run/secrets/kubernetes.io/serviceaccount/token

# JWT 디코딩으로 클레임 확인 (base64 -d로 페이로드 부분)
TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
echo $TOKEN | cut -d. -f2 | base64 -d 2>/dev/null | python3 -m json.tool
# 출력 예:
# {
#   "iss": "kubernetes/serviceaccount",
#   "kubernetes.io/serviceaccount/namespace": "default",
#   "kubernetes.io/serviceaccount/service-account.name": "my-sa",
#   "sub": "system:serviceaccount:default:my-sa",
#   "exp": 1754000000,   ← 만료 시간 (Bound Token은 1시간 기본)
#   "kubernetes.io/pod/name": "my-pod",  ← 어떤 파드에 바인딩됐는지
#   "kubernetes.io/pod/uid": "abc-123"
# }
```

#### ③ OIDC (OpenID Connect) — 외부 IDP 연동

```
사용자 → 외부 IDP(Google, Keycloak, Dex) → ID Token(JWT) → API Server
┌──────────────────────────────────────────────────────┐
│ 1. 사용자가 IDP에서 로그인 후 ID Token 획득           │
│                                                      │
│ 2. kubectl이 --token 또는 kubeconfig에 토큰 설정     │
│                                                      │
│ 3. API Server 검증:                                  │
│    - --oidc-issuer-url로 IDP의 JWKS 엔드포인트 조회  │
│    - JWKS의 공개키로 JWT 서명 검증                   │
│    - iss, aud 클레임 확인                            │
│    - --oidc-username-claim (보통 email) → 사용자명   │
│    - --oidc-groups-claim → 그룹                     │
└──────────────────────────────────────────────────────┘
```

```bash
# API Server의 OIDC 설정 확인 (kube-apiserver 파드 스펙)
kubectl get pod -n kube-system kube-apiserver-<노드명> -o yaml | grep oidc
# 주요 플래그:
# --oidc-issuer-url=https://accounts.google.com
# --oidc-client-id=kubernetes
# --oidc-username-claim=email
# --oidc-groups-claim=groups
```

#### ④ Bootstrap Token (노드 최초 클러스터 조인)

```
kubeadm join 흐름
┌──────────────────────────────────────────────────┐
│ 1. kubeadm init 시 Bootstrap Token 생성          │
│    (kube-system 네임스페이스의 Secret으로 저장)   │
│                                                  │
│ 2. 새 노드에서 kubeadm join --token <token>      │
│                                                  │
│ 3. API Server가 Bootstrap Token 검증             │
│    → system:bootstrappers 그룹으로 임시 인증      │
│                                                  │
│ 4. CSR(Certificate Signing Request) 자동 승인    │
│    → kubelet 클라이언트 인증서 발급              │
│                                                  │
│ 5. 이후 kubelet은 X.509 인증서로 통신            │
└──────────────────────────────────────────────────┘
```

---

## 3. etcd 저장 구조 심층 분석

### 3.1 etcd에 실제로 저장되는 방식

Kubernetes는 etcd를 **키-값 저장소**로 사용합니다. 키는 계층형 경로 구조를 따릅니다.

```
/registry/{resource-type}/{namespace}/{name}

예시:
/registry/pods/default/my-pod
/registry/deployments/production/my-deploy
/registry/secrets/kube-system/my-secret
/registry/serviceaccounts/default/my-sa
/registry/namespaces/default
/registry/nodes/worker-1
```

```bash
# etcd에서 직접 키 목록 조회 (etcdctl)
ETCDCTL_API=3 etcdctl \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key \
  get /registry --prefix --keys-only

# 특정 파드 데이터 조회
ETCDCTL_API=3 etcdctl \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key \
  get /registry/pods/default/my-pod
```

### 3.2 etcd에 저장되는 데이터 포맷

API Server는 etcd에 저장할 때 **Protocol Buffers**로 직렬화합니다 (JSON이 아님).

```
kubectl apply (JSON/YAML)
        │
        ▼
   API Server
        │  protobuf 직렬화 + 버전 변환 (내부 타입)
        ▼
      etcd 저장
        │
        │  protobuf 역직렬화 + 버전 변환 (요청 API 버전)
        ▼
   kubectl 응답 (JSON)
```

```bash
# etcd 값은 바이너리이므로 직접 읽으면 깨짐
# API Server를 통해 해석된 값 확인:
kubectl get pod my-pod -o json

# 실제 etcd 저장 키의 resourceVersion 확인
kubectl get pod my-pod -o jsonpath='{.metadata.resourceVersion}'
# 이 값이 etcd의 revision 번호와 대응됨
```

### 3.3 Secret의 etcd 저장과 암호화

기본적으로 Secret은 **Base64 인코딩**만 된 채 etcd에 평문 저장됩니다.

```
Secret 저장 흐름 (암호화 없는 기본):
kubectl create secret → API Server → etcd에 Base64만 인코딩된 값 저장

Secret 저장 흐름 (Encryption at Rest 활성화 시):
kubectl create secret → API Server
  → EncryptionConfiguration에 따라 AES-CBC/AES-GCM/KMS로 암호화
  → 암호화된 바이너리를 etcd에 저장
```

```yaml
# /etc/kubernetes/enc/encryption-config.yaml
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
  - resources:
      - secrets
    providers:
      - aescbc:                     # AES-CBC 256비트 암호화
          keys:
            - name: key1
              secret: <base64로 인코딩된 32바이트 키>
      - identity: {}                # 암호화 없음 (기존 데이터 읽기용)
```

```bash
# API Server에 암호화 설정 적용 (kube-apiserver 플래그)
# --encryption-provider-config=/etc/kubernetes/enc/encryption-config.yaml

# 기존 Secret을 모두 재암호화 (설정 변경 후)
kubectl get secrets --all-namespaces -o json \
  | kubectl replace -f -

# etcd에서 암호화 여부 확인 (k8s:enc:aescbc:v1: 접두사 확인)
ETCDCTL_API=3 etcdctl get /registry/secrets/default/my-secret \
  | hexdump -C | head -2
# 암호화 됐으면: 6b38733a656e633a61657363626... (k8s:enc:aescbc:v1:...)
# 평문이면:     6b38733a006974656d7320... (k8s: 이후 바로 protobuf)
```

### 3.4 Watch 메커니즘 — API Server가 etcd를 폴링하지 않는 이유

Controller, Scheduler 등이 리소스 변경을 감지하는 방식입니다.

```
Watch 흐름 (Long-polling 기반 HTTP/2 스트리밍):
┌──────────────────────────────────────────────────────┐
│ 1. 클라이언트(컨트롤러)가 Watch 요청                 │
│    GET /api/v1/pods?watch=true&resourceVersion=12345  │
│                                                      │
│ 2. API Server가 etcd에 Watch 등록                    │
│    (etcd의 watch는 gRPC 스트리밍)                    │
│                                                      │
│ 3. etcd에 변경 발생 시 이벤트 스트리밍               │
│    etcd → API Server → 모든 Watch 구독자에게 전파    │
│                                                      │
│ 4. API Server 내부 캐시(Watch Cache)가               │
│    etcd 부하를 줄여줌                                │
│    (List 요청도 캐시에서 서빙)                       │
└──────────────────────────────────────────────────────┘

이벤트 타입:
- ADDED   : 새 리소스 생성
- MODIFIED: 기존 리소스 변경 (resourceVersion 증가)
- DELETED : 리소스 삭제
```

```bash
# Watch 직접 확인
kubectl get pods --watch

# 특정 resourceVersion 이후의 변경만 수신
kubectl get pods --watch --resource-version=<버전번호>

# API 레벨에서 Watch 이벤트 확인
kubectl get pods -w -o json 2>/dev/null | jq '{type: .type, name: .object.metadata.name}'
```

### 3.5 resourceVersion과 낙관적 동시성 제어

```
동시 수정 충돌 방지 메커니즘:
┌──────────────────────────────────────────────────────────┐
│ 1. 클라이언트 A: GET pod/my-pod → resourceVersion: 100  │
│ 2. 클라이언트 B: GET pod/my-pod → resourceVersion: 100  │
│                                                          │
│ 3. 클라이언트 A: PUT pod/my-pod (resourceVersion: 100)  │
│    → etcd에서 현재 revision이 100인지 확인 후 저장       │
│    → 성공, resourceVersion: 101로 증가                  │
│                                                          │
│ 4. 클라이언트 B: PUT pod/my-pod (resourceVersion: 100)  │
│    → etcd에서 현재 revision이 101 (불일치)               │
│    → 409 Conflict 반환                                   │
│    → 클라이언트 B는 다시 GET → PUT 재시도               │
└──────────────────────────────────────────────────────────┘
```

---

## 4. 트러블슈팅

* **`Unauthorized (401)` 오류:**
  ```bash
  # 현재 kubeconfig의 인증 정보 확인
  kubectl config view --minify
  # 토큰/인증서 만료 여부 확인
  kubectl auth can-i get pods --as=system:serviceaccount:default:my-sa
  ```

* **Service Account 토큰이 만료됨 (1시간 후 자동 갱신):**
  ```bash
  # kubelet이 자동 갱신하므로 실제로는 문제없음
  # 갱신 안 되는 경우 kubelet 로그 확인
  journalctl -u kubelet | grep "token"
  ```

* **etcd 데이터 백업:**
  ```bash
  ETCDCTL_API=3 etcdctl snapshot save /backup/etcd-snapshot.db \
    --endpoints=https://127.0.0.1:2379 \
    --cacert=/etc/kubernetes/pki/etcd/ca.crt \
    --cert=/etc/kubernetes/pki/etcd/healthcheck-client.crt \
    --key=/etc/kubernetes/pki/etcd/healthcheck-client.key

  # 복구
  ETCDCTL_API=3 etcdctl snapshot restore /backup/etcd-snapshot.db \
    --data-dir=/var/lib/etcd-restore
  ```

* **etcd 용량 부족 (`mvcc: database space exceeded`):**
  ```bash
  # 컴팩션 (오래된 리비전 정리)
  ETCDCTL_API=3 etcdctl compact $(ETCDCTL_API=3 etcdctl endpoint status \
    --write-out="json" | jq -r '.[0].Status.header.revision')

  # 조각 모음
  ETCDCTL_API=3 etcdctl defrag --endpoints=https://127.0.0.1:2379 \
    --cacert=... --cert=... --key=...
  ```
