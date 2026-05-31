## 1. 개요 및 비유

Kubernetes 클러스터는 내부 통신 전체를 TLS로 암호화하며, 이를 위한 독자적인 PKI(Public Key Infrastructure) 체계를 가집니다.

💡 **비유하자면 회사 내부 보안 출입 시스템과 같습니다.**
회사 자체 CA(인증서 발급 기관)가 직원증(인증서)을 발급하고, 모든 문(API Server, etcd, kubelet)은 이 직원증을 검증합니다. 외부 공인 인증서가 필요 없습니다.

---

## 2. 클러스터 PKI 전체 구조

### 2.1 인증서 계층 및 용도

```
/etc/kubernetes/pki/
├── ca.crt / ca.key                 ← 클러스터 루트 CA
│   서명 대상:
│   ├── apiserver.crt               ← API Server 서버 인증서
│   ├── apiserver-kubelet-client.crt← API Server → kubelet 클라이언트 인증서
│   ├── front-proxy-ca.crt/key      ← API Aggregation용 별도 CA
│   │   └── front-proxy-client.crt
│   └── (kubelet이 CSR로 요청한 인증서들)
│
├── etcd/
│   ├── ca.crt / ca.key             ← etcd 전용 CA (별도 CA!)
│   │   서명 대상:
│   │   ├── server.crt              ← etcd 서버 인증서
│   │   ├── peer.crt                ← etcd 피어 간 통신 인증서
│   │   └── healthcheck-client.crt  ← 상태 체크용 클라이언트 인증서
│   └── apiserver-etcd-client.crt   ← API Server → etcd 클라이언트 인증서
│
└── sa.key / sa.pub                 ← Service Account 토큰 서명 키쌍
    (인증서가 아닌 RSA 키쌍)
```

### 2.2 각 컴포넌트 간 인증서 사용 매핑

```
컴포넌트 간 TLS 연결 요약:

kubectl → API Server:
  - 서버 검증: ca.crt로 apiserver.crt 검증
  - 클라이언트 인증: kubeconfig의 client-certificate (X.509 또는 토큰)

API Server → etcd:
  - 서버 검증: etcd/ca.crt로 etcd/server.crt 검증
  - 클라이언트 인증: apiserver-etcd-client.crt (etcd/ca.crt 서명)

API Server → kubelet:
  - 서버 검증: ca.crt로 kubelet.crt 검증
  - 클라이언트 인증: apiserver-kubelet-client.crt

kubelet → API Server:
  - 서버 검증: ca.crt (클러스터 CA)
  - 클라이언트 인증: kubelet-client.crt (CN=system:node:<hostname>)

etcd peer ↔ peer:
  - 상호 검증: etcd/ca.crt로 etcd/peer.crt 검증
```

```bash
# 클러스터 CA 정보 확인
openssl x509 -in /etc/kubernetes/pki/ca.crt -noout -text | grep -E "Subject:|Not After"

# API Server 인증서의 SAN(Subject Alternative Names) 확인
openssl x509 -in /etc/kubernetes/pki/apiserver.crt -noout -ext subjectAltName
# 출력 예:
# DNS:kubernetes, DNS:kubernetes.default, DNS:kubernetes.default.svc,
# DNS:kubernetes.default.svc.cluster.local, DNS:master-1,
# IP:10.96.0.1, IP:192.168.1.10

# 인증서 만료일 전체 조회
kubeadm certs check-expiration
# 또는
for cert in /etc/kubernetes/pki/*.crt; do
  echo "=== $cert ===";
  openssl x509 -in $cert -noout -dates 2>/dev/null;
done
```

---

## 3. 인증서 갱신 (Certificate Rotation)

### 3.1 kubeadm 인증서 갱신

kubeadm으로 만든 클러스터의 인증서는 기본 **1년** 유효기간입니다.

```bash
# 전체 인증서 만료일 확인
kubeadm certs check-expiration
# 출력 예:
# CERTIFICATE                EXPIRES                  RESIDUAL TIME
# admin.conf                 Jan 01, 2026 00:00 UTC   364d
# apiserver                  Jan 01, 2026 00:00 UTC   364d
# apiserver-etcd-client      Jan 01, 2026 00:00 UTC   364d
# ...

# 전체 인증서 갱신 (컨트롤 플레인에서 실행)
kubeadm certs renew all

# 갱신 후 컨트롤 플레인 컴포넌트 재시작 (static pod)
# /etc/kubernetes/manifests/ 내 파드들이 자동 재시작됨
# (파일 수정 또는 kubelet 재시작)
systemctl restart kubelet

# 새 kubeconfig로 kubectl 업데이트
cp /etc/kubernetes/admin.conf ~/.kube/config
```

### 3.2 kubelet 클라이언트 인증서 자동 갱신

kubelet은 인증서 만료 전에 **자동으로 CSR(Certificate Signing Request)을 생성**합니다.

```
자동 갱신 흐름:
만료 80% 도달 시:
  1. kubelet이 새 키쌍 생성
  2. CSR 오브젝트 생성 → API Server에 제출
  3. kube-controller-manager의 CSR Auto Approver가 자동 승인
  4. 새 인증서 발급 → /var/lib/kubelet/pki/ 에 저장
  5. 이전 인증서와 교체
```

```bash
# kubelet 자동 갱신 설정 확인
cat /var/lib/kubelet/config.yaml | grep -i rotate
# rotateCertificates: true  (기본값)

# 현재 kubelet 클라이언트 인증서 만료일 확인
openssl x509 -in /var/lib/kubelet/pki/kubelet-client-current.pem \
  -noout -dates

# CSR 목록 확인 (자동 갱신 시 생성됨)
kubectl get csr
# Approved,Issued 상태여야 함
```

---

## 4. cert-manager — 애플리케이션 인증서 자동 관리

### 4.1 cert-manager 구조

```
cert-manager Controller (클러스터 내 Deployment):
  ┌─────────────────────────────────────────────┐
  │  Certificate 오브젝트 감시                   │
  │  → Issuer/ClusterIssuer에 인증서 요청        │
  │  → 발급된 인증서를 Secret으로 저장           │
  │  → 만료 전 자동 갱신                        │
  └─────────────────────────────────────────────┘

Issuer 종류:
- SelfSigned  : 자체 서명 (테스트용)
- CA          : 클러스터 내 CA Secret 사용
- ACME (Let's Encrypt): HTTP-01 / DNS-01 챌린지
- Vault       : HashiCorp Vault 연동
- Venafi      : 엔터프라이즈 PKI 연동
```

```yaml
# ClusterIssuer — Let's Encrypt 연동
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: admin@mycompany.com
    privateKeySecretRef:
      name: letsencrypt-prod-key
    solvers:
    - http01:
        ingress:
          class: nginx

---
# Certificate 요청
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: my-app-tls
  namespace: production
spec:
  secretName: my-app-tls-secret   # 발급된 인증서가 저장될 Secret
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
  dnsNames:
  - myapp.example.com
  - www.myapp.example.com
  duration: 2160h     # 90일 (Let's Encrypt 최대)
  renewBefore: 360h   # 만료 15일 전에 갱신

---
# Ingress에서 자동 인증서 사용
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-app-ingress
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
  - hosts: [myapp.example.com]
    secretName: my-app-tls-secret
  rules:
  - host: myapp.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: my-app
            port:
              number: 80
```

### 4.2 클러스터 내부 mTLS (서비스 간 상호 인증)

```yaml
# 내부 CA로 서비스 간 mTLS 인증서 발급
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: my-service-mtls
spec:
  secretName: my-service-mtls-secret
  issuerRef:
    name: internal-ca-issuer
    kind: ClusterIssuer
  usages:
  - server auth
  - client auth       # 클라이언트 인증서로도 사용
  dnsNames:
  - my-service.default.svc.cluster.local
```

---

## 5. 트러블슈팅

* **인증서 만료로 kubectl 작동 불가:**
  ```bash
  # 에러: x509: certificate has expired or is not yet valid

  # 1. 임시 방편: kubeconfig에서 인증서 검증 무시 (권장하지 않음)
  kubectl --insecure-skip-tls-verify get nodes

  # 2. 올바른 해결: kubeadm으로 갱신
  kubeadm certs renew all
  cp /etc/kubernetes/admin.conf ~/.kube/config
  systemctl restart kubelet
  ```

* **cert-manager가 인증서 발급 실패:**
  ```bash
  # Certificate 상태 확인
  kubectl describe certificate my-app-tls
  # Events에서 실패 이유 확인

  # CertificateRequest 확인
  kubectl get certificaterequest
  kubectl describe certificaterequest my-app-tls-xxxxx

  # ACME 챌린지 상태 확인
  kubectl get challenges
  kubectl describe challenge <challenge-name>
  # HTTP-01이면 Ingress 경로 접근 가능한지 확인
  curl http://myapp.example.com/.well-known/acme-challenge/<token>
  ```

* **새 노드 추가 후 kubelet 인증서 문제:**
  ```bash
  # Bootstrap Token이 유효한지 확인
  kubectl get secret -n kube-system | grep bootstrap-token

  # CSR 수동 승인
  kubectl get csr
  kubectl certificate approve <csr-name>
  ```
