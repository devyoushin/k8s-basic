## 1. 개요 및 비유

`.kube/config`는 kubectl이 API Server에 접근하기 위한 **신분증 + 지도 + 열쇠 묶음**입니다.

💡 **비유하자면 은행 OTP 카드 + 지점 주소 + 계좌번호 세트와 같습니다.**
- 어느 은행 지점(cluster endpoint)에 갈지,
- 내 계좌가 무엇인지(user identity),
- 어떤 OTP로 인증할지(credentials)
를 한 파일에 담고 있으며, 지점마다 다른 세트를 context로 전환해 사용합니다.

---

## 2. kubeconfig 파일 구조 심층 분석

### 2.1 전체 구조

```yaml
apiVersion: v1
kind: Config

# ① 어디에 붙을지 — API Server 주소 + 신뢰할 CA
clusters:
  - name: my-cluster
    cluster:
      server: https://ABCDEF1234567890.gr7.ap-northeast-2.eks.amazonaws.com  # EKS 엔드포인트
      certificate-authority-data: BASE64(ca.crt)  # API Server TLS 검증용 클러스터 CA

# ② 누구인가 — 인증 방식 선택
users:
  - name: my-iam-user
    user:
      # 방식 A: 클라이언트 인증서 (kubeadm 클러스터)
      client-certificate-data: BASE64(client.crt)
      client-key-data: BASE64(client.key)

      # 방식 B: Bearer 토큰 (ServiceAccount / OIDC)
      token: eyJhbGciOiJSUzI1NiIsInR5...

      # 방식 C: exec credential plugin (EKS, GKE 등)
      exec:
        apiVersion: client.authentication.k8s.io/v1beta1
        command: aws
        args: ["eks", "get-token", "--cluster-name", "my-cluster", "--region", "ap-northeast-2"]
        interactiveMode: IfAvailable

# ③ ①+②를 묶어 하나의 접근 세트로
contexts:
  - name: my-context
    context:
      cluster: my-cluster
      user: my-iam-user
      namespace: default  # 생략 시 default

current-context: my-context
```

### 2.2 핵심 필드 의미

| 필드 | 역할 | 없으면 |
|------|------|--------|
| `server` | TCP 연결 대상 | 연결 불가 |
| `certificate-authority-data` | 서버 TLS 인증서 검증 | MITM 취약 or 연결 거부 |
| `client-certificate-data` + `client-key-data` | mTLS 클라이언트 인증 | 다른 인증 방식 필요 |
| `token` | Bearer 토큰 인증 | 다른 인증 방식 필요 |
| `exec` | 외부 프로그램으로 토큰 동적 발급 | EKS 접근 불가 |

---

## 3. kubectl → API Server 통신 전체 흐름

### 3.1 단계별 흐름

```
kubectl get pods
    │
    ▼
[1] kubeconfig 로드
    ~/.kube/config 파싱 → current-context 확인
    cluster.server = "https://..."
    user.exec = { command: "aws", args: [...] }
    │
    ▼
[2] Credential 획득
    exec 플러그인이 있으면 → 외부 명령 실행해 토큰 받아옴
    캐시된 토큰이 유효하면 재사용 (expirationTimestamp 확인)
    │
    ▼
[3] TLS 연결 수립
    TCP SYN → SYN-ACK → ACK
    ClientHello → ServerHello
    API Server가 서버 인증서 제시
    kubectl이 kubeconfig의 CA로 검증
    (mTLS면 클라이언트 인증서도 제시)
    │
    ▼
[4] HTTP/2 요청 전송
    GET /api/v1/namespaces/default/pods HTTP/2
    Authorization: Bearer <token>
    │
    ▼
[5] API Server 인증(Authentication)
    어떤 인증 방식인가 순서대로 시도:
    X.509 cert → Bearer token → OIDC → Webhook → ...
    결과: username, groups, extra attributes
    │
    ▼
[6] 인가(Authorization) — RBAC
    이 username/groups가 이 리소스에 이 verb를 할 수 있는가?
    │
    ▼
[7] Admission Control
    MutatingWebhook → ValidatingWebhook
    │
    ▼
[8] etcd 조회/저장 후 응답 반환
```

### 3.2 TLS Handshake 상세

```
kubectl                           API Server
  │                                   │
  │──── TCP Connect (6443) ──────────▶│
  │                                   │
  │──── ClientHello ─────────────────▶│
  │         (지원 TLS 버전, cipher suites)
  │                                   │
  │◀─── ServerHello ──────────────────│
  │         (선택된 cipher)           │
  │◀─── Certificate ──────────────────│
  │         (API Server의 인증서)     │
  │◀─── ServerHelloDone ──────────────│
  │                                   │
  │  [CA 검증] kubeconfig의           │
  │  certificate-authority-data로     │
  │  서버 인증서 서명 체인 검증        │
  │                                   │
  │──── ClientKeyExchange ───────────▶│
  │──── [ChangeCipherSpec] ──────────▶│
  │──── Finished ────────────────────▶│
  │◀─── [ChangeCipherSpec] ────────────│
  │◀─── Finished ──────────────────────│
  │                                   │
  │  (mTLS인 경우 위 중간에 클라이언트│
  │   인증서도 교환)                  │
  │                                   │
  │══════ 암호화 채널 확립 ═══════════│
  │──── GET /api/v1/namespaces/... ──▶│
```

---

## 4. API Server → Kubelet 통신 흐름

kubectl이 `exec`, `logs`, `port-forward` 같은 명령을 내리면 API Server가 직접 kubelet에 요청합니다.

### 4.1 왜 API Server가 Kubelet에 연결하는가?

```
일반 조회 (get pods, get deployments):
  kubectl → API Server → etcd (kubelet 관여 없음)

실시간/양방향 명령:
  kubectl exec/logs/port-forward
  kubectl → API Server → Kubelet
  (API Server가 proxy/upgrade 역할)
```

### 4.2 API Server → Kubelet 인증

```
/etc/kubernetes/pki/
├── apiserver-kubelet-client.crt  ← API Server가 kubelet에 제시하는 클라이언트 인증서
└── apiserver-kubelet-client.key  ← 해당 개인키
```

```
API Server                        Kubelet (10250 포트)
  │                                   │
  │──── TLS 연결 + 클라이언트 인증서 ▶│
  │     (CN=kube-apiserver-kubelet-   │
  │      client, O=system:masters)    │
  │                                   │
  │  [Kubelet 검증]                   │
  │  --client-ca-file=ca.crt 로       │
  │  API Server 클라이언트 인증서 검증 │
  │                                   │
  │──── GET /pods ───────────────────▶│
  │──── POST /exec/<pod>/<container> ▶│
```

### 4.3 Kubelet 서버 인증서 검증

API Server가 kubelet TLS를 어떻게 신뢰하는가:

```
방식 1: --kubelet-certificate-authority 플래그
  API Server 시작 시 kubelet CA 지정

방식 2: kubelet TLS Bootstrapping
  kubelet이 CSR(Certificate Signing Request)를 API Server에 제출
  → kube-controller-manager가 자동 서명 (kubelet-serving 그룹)
  → kubelet이 자체 서버 인증서를 클러스터 CA로 발급받음

방식 3 (위험): --insecure-skip-tls-verify (검증 생략, 비권장)
```

### 4.4 exec/logs 실제 프로토콜

```
kubectl exec -it pod-name -- bash

[1] kubectl → API Server
    POST /api/v1/namespaces/default/pods/pod-name/exec
    ?command=bash&stdin=true&stdout=true&tty=true
    Connection: Upgrade
    Upgrade: SPDY/3.1  (또는 WebSocket)

[2] API Server → Kubelet
    POST https://<node-ip>:10250/exec/<namespace>/<pod>/<container>
    (동일 파라미터 전달)
    클라이언트 인증서로 mTLS

[3] Kubelet → Container Runtime (CRI)
    ContainerExec RPC → containerd → runc
    → 컨테이너 내 프로세스 실행

[4] 양방향 스트리밍
    kubectl ←────SPDY/WebSocket stream──── API Server ←── Kubelet ←── container
    stdin/stdout/stderr 동시 다중화
```

---

## 5. EKS에서 kubeconfig와 API 요청이 가능해지는 원리

### 5.1 EKS 인증 아키텍처 전체 그림

```
                    ┌──────────────────────────┐
                    │   AWS IAM 서비스          │
                    │  (STS: sts.amazonaws.com) │
                    └──────────┬───────────────┘
                               │ PresignedURL 서명/검증
    ┌──────────────────────────▼───────────────────────────────┐
    │                   EKS Control Plane                       │
    │                                                           │
    │  ┌─────────────────┐    ┌──────────────────────────────┐ │
    │  │   API Server    │───▶│  aws-iam-authenticator       │ │
    │  │  (관리형, AWS)   │    │  (Webhook Token Authenticator│ │
    │  └────────┬────────┘    └──────────────────────────────┘ │
    │           │                                               │
    └───────────┼───────────────────────────────────────────────┘
                │
    ┌───────────▼───────────────┐
    │   aws-auth ConfigMap      │
    │   (kube-system namespace) │
    │   IAM ARN → K8s RBAC 매핑 │
    └───────────────────────────┘
```

### 5.2 EKS 토큰 발급 흐름 (exec credential)

```
kubectl get pods
    │
    ▼
[1] kubeconfig의 exec 섹션 실행
    $ aws eks get-token --cluster-name my-cluster --region ap-northeast-2

    내부 동작:
    aws CLI → AWS SDK → STS.GetCallerIdentity API 호출을
    실제 실행하지 않고 "presigned URL"로 만듦

    presigned URL =
      https://sts.amazonaws.com/
      ?Action=GetCallerIdentity
      &X-Amz-Algorithm=AWS4-HMAC-SHA256
      &X-Amz-Credential=AKID.../aws4_request
      &X-Amz-Date=...
      &X-Amz-Expires=60           ← 60초 유효
      &X-Amz-SignedHeaders=host;x-k8s-aws-id
      &x-k8s-aws-id=my-cluster    ← 클러스터 지정 (다른 클러스터 재사용 방지)
      &X-Amz-Signature=...        ← IAM 자격증명으로 서명

    이 URL을 Base64 URL-safe encode → Bearer 토큰 문자열 (k8s-aws-v1.XXXX)
    │
    ▼
[2] kubectl → EKS API Server
    Authorization: Bearer k8s-aws-v1.BASE64(presignedURL)
    │
    ▼
[3] EKS API Server → aws-iam-authenticator (Webhook)
    TokenReview 요청:
    POST /authenticate
    { "token": "k8s-aws-v1.BASE64(presignedURL)" }
    │
    ▼
[4] aws-iam-authenticator 처리
    Base64 decode → presigned URL 복원
    → STS 엔드포인트에 실제 GET 요청 (URL 자체가 인증 증명)
    → STS 응답: { "Account": "123456789", "Arn": "arn:aws:iam::123456789:user/sunny", "UserId": "AIDAXXXXXXXX" }
    → IAM Identity 확인 완료
    │
    ▼
[5] aws-auth ConfigMap 조회
    arn:aws:iam::123456789:user/sunny
      → K8s username: sunny
         groups: [system:masters]  (또는 커스텀 그룹)
    │
    ▼
[6] API Server에 TokenReview 응답
    { "authenticated": true, "user": { "username": "sunny", "groups": ["system:masters"] } }
    │
    ▼
[7] RBAC 인가 처리 (일반 K8s와 동일)
```

### 5.3 aws-auth ConfigMap 구조

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: aws-auth
  namespace: kube-system
data:
  # IAM 역할(Role) 매핑 — EC2 노드 그룹, 파드 ServiceAccount IRSA 등
  mapRoles: |
    - rolearn: arn:aws:iam::123456789:role/eks-node-group-role
      username: system:node:{{EC2PrivateDNSName}}   # 노드 kubelet 신원
      groups:
        - system:bootstrappers
        - system:nodes

    - rolearn: arn:aws:iam::123456789:role/eks-admin-role
      username: eks-admin
      groups:
        - system:masters  # cluster-admin 수준

  # IAM 사용자(User) 직접 매핑
  mapUsers: |
    - userarn: arn:aws:iam::123456789:user/sunny
      username: sunny
      groups:
        - system:masters

    - userarn: arn:aws:iam::123456789:user/developer
      username: developer
      groups:
        - dev-team  # 커스텀 그룹 → 별도 RoleBinding 필요
```

### 5.4 EKS 클러스터 생성자(Creator)의 특수 권한

```
⚠️ 주의: EKS 클러스터를 최초 생성한 IAM 엔티티(User/Role)는
aws-auth ConfigMap에 등록하지 않아도 자동으로 system:masters 권한 보유

이유: 부트스트랩 시 API Server 내부에 하드코딩된 예외
→ 클러스터 생성 직후 aws-auth 설정을 위해 필요

보안 권장사항:
- CI/CD 파이프라인 Role로 생성하지 말 것 (해당 Role 삭제 시 접근 불가)
- 전용 관리 IAM Role을 생성자로 사용하고 aws-auth에도 등록
```

---

## 6. Bastion 서버에서 EKS 클러스터 제어에 필요한 권한

### 6.1 필요한 권한 레이어 구분

```
레이어 1: OS 레벨
  - aws CLI 설치
  - kubectl 설치
  - ~/.kube/config 존재 (또는 aws eks update-kubeconfig 실행 권한)

레이어 2: AWS IAM 권한 (AWS API 호출)
  - STS: 토큰 생성 (presigned URL 서명)
  - EKS: kubeconfig 업데이트

레이어 3: K8s RBAC 권한 (aws-auth ConfigMap 매핑)
  - Bastion의 IAM Role/User가 aws-auth에 매핑되어 있어야 함

레이어 4: 네트워크 접근
  - EKS API Server 엔드포인트에 TCP 6443 접근 가능해야 함
```

### 6.2 필요한 IAM 정책 (최소 권한)

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "EKSDescribeAndToken",
      "Effect": "Allow",
      "Action": [
        "eks:DescribeCluster",       // kubeconfig 생성 시 엔드포인트/CA 조회
        "eks:ListClusters"           // 클러스터 목록 조회 (선택)
      ],
      "Resource": "arn:aws:iam::123456789:cluster/my-cluster"
    },
    {
      "Sid": "STSForTokenGeneration",
      "Effect": "Allow",
      "Action": [
        "sts:GetCallerIdentity"      // aws-iam-authenticator가 호출하는 API
      ],
      "Resource": "*"
    }
  ]
}
```

> `sts:GetCallerIdentity`는 실제로 IAM 정책에서 명시적으로 허용하지 않아도 모든 인증된 IAM 엔티티가 호출 가능합니다. 그러나 **STS 엔드포인트 접근 가능** (VPC 내 STS VPC Endpoint 또는 인터넷 경로) 이 필요합니다.

### 6.3 kubeconfig 자동 생성

```bash
# Bastion에서 실행 — EKS 클러스터 정보를 읽어 ~/.kube/config 자동 생성/갱신
aws eks update-kubeconfig \
  --region ap-northeast-2 \
  --name my-cluster \
  --role-arn arn:aws:iam::123456789:role/eks-admin-role  # assume할 Role 지정 (선택)

# 결과: ~/.kube/config에 아래 내용 추가됨
# clusters[].cluster.server = EKS 엔드포인트
# clusters[].cluster.certificate-authority-data = 클러스터 CA
# users[].user.exec = aws eks get-token ...
```

### 6.4 aws-auth 매핑 설정

```yaml
# Bastion에서 사용할 IAM Role을 aws-auth에 추가
# (이 작업 자체는 이미 cluster-admin 권한 가진 사람이 수행)
data:
  mapRoles: |
    - rolearn: arn:aws:iam::123456789:role/bastion-eks-role
      username: bastion-admin
      groups:
        - system:masters   # 전체 관리자

    # 읽기 전용 Bastion이라면:
    - rolearn: arn:aws:iam::123456789:role/bastion-readonly-role
      username: bastion-reader
      groups:
        - view-only        # ClusterRole view에 바인딩된 커스텀 그룹
```

### 6.5 네트워크 접근 요건

```
EKS API Server 엔드포인트 접근 방식:

방식 1: Public Endpoint (기본값)
  Bastion → 인터넷 → EKS Public Endpoint (0.0.0.0/0 또는 허용 CIDR)
  설정: eks.update-cluster-config --resources-vpc-config publicAccessCidrs=x.x.x.x/32

방식 2: Private Endpoint
  Bastion(VPC 내) → VPC 내부 → EKS Private Endpoint
  요건:
  - Bastion이 EKS 클러스터와 동일 VPC 또는 Peering/TGW로 연결
  - Security Group: Bastion → EKS SG, TCP 443 인바운드 허용
  - Route Table: EKS 서브넷 경로 존재

방식 3: Public + Private 혼합 (권장)
  - VPC 내부 트래픽은 Private Endpoint로
  - 외부 접근은 허용 CIDR로 제한된 Public Endpoint로

포트 정리:
  443 (HTTPS)  → EKS API Server (kubectl 통신)
  10250        → Kubelet (API Server → Node, Bastion 직접 접근 불필요)
  2379-2380    → etcd (외부 노출 안 함, 관리형 EKS에서 접근 불가)
```

### 6.6 Bastion에서 EKS 접근 전체 체크리스트

```
[ ] aws CLI 설치 및 IAM 자격증명 설정
    - EC2 Instance Profile 또는 ~/.aws/credentials
    - aws sts get-caller-identity 로 확인

[ ] kubectl 설치
    - kubectl version --client

[ ] kubeconfig 생성
    - aws eks update-kubeconfig --name <cluster> --region <region>
    - kubectl config current-context

[ ] AWS IAM 권한 확인
    - eks:DescribeCluster 권한
    - STS 엔드포인트 접근 가능 (VPC Endpoint 또는 인터넷)

[ ] aws-auth ConfigMap에 IAM 엔티티 등록 확인
    - kubectl get configmap aws-auth -n kube-system -o yaml

[ ] 네트워크 경로 확인
    - nc -zv <eks-endpoint> 443
    - Security Group 인바운드 규칙

[ ] 실제 접근 테스트
    - kubectl get nodes
    - kubectl auth can-i list pods --namespace default
```

---

## 7. EKS 이후 — IRSA와 파드 내 인증

### 7.1 IRSA (IAM Roles for Service Accounts)

파드 내부에서도 동일한 OIDC 기반 인증을 사용합니다:

```
파드 (ServiceAccount 토큰 마운트됨)
  /var/run/secrets/eks.amazonaws.com/serviceaccount/token
  (OIDC JWT 토큰 — 짧은 유효기간)
  │
  ▼
AWS SDK → STS.AssumeRoleWithWebIdentity
  token=<OIDC JWT>
  roleArn=arn:aws:iam::123456789:role/my-pod-role
  │
  ▼
STS → EKS OIDC Provider에 JWT 검증 요청
  https://oidc.eks.ap-northeast-2.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE
  │
  ▼
임시 AWS 자격증명 발급 (AccessKey + SecretKey + SessionToken)
```

---

## 8. 트러블슈팅

### 8.1 `error: You must be logged in to the server (Unauthorized)`

```bash
# 원인 1: aws-auth에 IAM 엔티티 미등록
kubectl get configmap aws-auth -n kube-system -o yaml
# → mapRoles 또는 mapUsers에 현재 IAM ARN 확인

# 원인 2: 토큰 만료 (60초) — 보통 자동 갱신되지만
aws eks get-token --cluster-name <name> --region <region>
# → 직접 토큰 발급해 만료 여부 확인

# 원인 3: 잘못된 AWS Identity로 토큰 발급
aws sts get-caller-identity
# → 예상한 IAM User/Role인지 확인
```

### 8.2 `Unable to connect to the server: dial tcp: i/o timeout`

```bash
# 원인: 네트워크 경로 없음
# 1. EKS 엔드포인트 확인
aws eks describe-cluster --name <name> --query 'cluster.endpoint'

# 2. 포트 연결 테스트
nc -zv <endpoint-host> 443
curl -k https://<endpoint>/healthz

# 3. Private Endpoint인 경우 VPC 내부에서만 접근 가능
# → Bastion이 올바른 VPC/서브넷에 있는지 확인
```

### 8.3 `error executing exec plugin: executable aws not found`

```bash
# kubeconfig exec.command = "aws" 이지만 PATH에 없음
which aws
export PATH=$PATH:/usr/local/bin  # aws CLI 설치 경로 추가

# 또는 절대 경로로 kubeconfig 수정
# command: /usr/local/bin/aws
```

### 8.4 `pods is forbidden: User "sunny" cannot list resource "pods"`

```bash
# 인증은 됐지만 RBAC 인가 실패
# aws-auth의 groups 확인 — system:masters 가 있어야 cluster-admin
kubectl get configmap aws-auth -n kube-system -o yaml | grep -A5 sunny

# 현재 권한 확인
kubectl auth can-i --list
```

### 8.5 Kubelet 통신 오류 (`Error from server: error dialing backend`)

```bash
# API Server → Kubelet 연결 실패
# 원인 1: 노드 Security Group에서 10250 포트 차단
# EKS 노드 SG에 Control Plane SG로부터 TCP 10250 인바운드 허용 확인

# 원인 2: 노드 내 kubelet 미동작
kubectl get node <node-name>  # NotReady 확인
# → EC2 콘솔에서 노드 상태 확인
```
