# AWS Private CA + cert-manager 인증서 계층 관리

## 1. 개요 및 비유

AWS Private Certificate Authority(PCA)와 cert-manager를 연동하여 조직 내 인증서 발급·갱신·폐기를 자동화하는 체계입니다.
비유: 국가(Root CA) → 은행(Intermediate CA) → 개인 통장(Leaf 인증서) 구조로, 각 도메인/조직이 독립적인 권한을 갖습니다.

---

## 2. 핵심 설명

### AWS PCA 계층 구조 설계

```
Root CA (오프라인 보관, HSM)
  ├── Intermediate CA — 인프라/메시
  │     └── Leaf: *.mesh.internal (Istio 워크로드 인증서)
  │     └── Leaf: *.internal (내부 서비스 간 통신)
  │
  ├── Intermediate CA — API 외부 노출
  │     └── Leaf: api.fintech.com (ALB/NLB 엔드포인트)
  │     └── Leaf: partner-api.fintech.com (B2B 파트너 API)
  │
  ├── Intermediate CA — 클라이언트 인증
  │     └── Leaf: mobile-client (모바일 앱 클라이언트 인증서)
  │     └── Leaf: admin-client (운영자 클라이언트 인증서)
  │
  └── Intermediate CA — 개발/스테이징
        └── Leaf: *.dev.internal
        └── Leaf: *.staging.internal
```

### 비즈니스 도메인별 Intermediate CA 분리 원칙

- **결제(Payments)**: 별도 Intermediate CA → PCI-DSS 감사 범위 격리
- **인증(Auth)**: 별도 Intermediate CA → 클라이언트 인증서 전용
- **인프라(Infra)**: Istio mesh, 내부 서비스 전용
- 분리 이유: 한 도메인 CA 침해 시 다른 도메인 영향 차단, 감사 로그 분리

### cert-manager Issuer 계층

```
ClusterIssuer (aws-pca-infra)     → Istio mesh 인증서
Issuer (payments/aws-pca-payments) → payments 네임스페이스 전용
Issuer (auth/aws-pca-client)       → 클라이언트 인증서 전용
```

### SSL Pinning과 Key Reuse 전략

SSL Pinning 환경에서 cert-manager 자동 갱신 시 Public Key가 변경되면 핀된 클라이언트 전원 통신 불가.

**해결책: `reusePrivateKey: true` 설정**
- 인증서 갱신 시 기존 Private Key 재사용 → Public Key 유지 → 핀 유지
- 단, 키 침해(Compromise) 의심 시 즉시 `reusePrivateKey: false`로 강제 갱신

---

## 3. YAML 적용 예시

### AWS PCA Issuer — 도메인별 분리 설정

```yaml
# 인프라/메시용 ClusterIssuer
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: aws-pca-infra
spec:
  acmpca:
    arn: arn:aws:acm-pca:ap-northeast-2:123456789:certificate-authority/infra-ca-id
    region: ap-northeast-2
    signingAlgorithm: SHA512WITHRSA
    template:
      arn: arn:aws:acm-pca:::template/EndEntityCertificate/V1
---
# 결제 도메인 전용 Issuer (payments 네임스페이스)
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: aws-pca-payments
  namespace: payments
spec:
  acmpca:
    arn: arn:aws:acm-pca:ap-northeast-2:123456789:certificate-authority/payments-ca-id
    region: ap-northeast-2
    signingAlgorithm: SHA512WITHRSA
---
# 클라이언트 인증서 전용 Issuer
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: aws-pca-client
  namespace: auth
spec:
  acmpca:
    arn: arn:aws:acm-pca:ap-northeast-2:123456789:certificate-authority/client-ca-id
    region: ap-northeast-2
    signingAlgorithm: SHA256WITHRSA
    template:
      arn: arn:aws:acm-pca:::template/EndEntityClientAuthCertificate/V1
```

### Certificate — SSL Pinning 환경용 (Key Reuse)

```yaml
# 외부 노출 API 인증서 (SSL Pinning 클라이언트 대상)
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: api-fintech-cert
  namespace: istio-system
spec:
  secretName: api-fintech-tls
  issuerRef:
    name: aws-pca-api
    kind: ClusterIssuer
  commonName: api.fintech.com
  dnsNames:
    - api.fintech.com
    - api-v2.fintech.com
  duration: 8760h   # 1년
  renewBefore: 720h # 30일 전 갱신 (앱 업데이트 협의 시간 확보)
  privateKey:
    algorithm: RSA
    size: 4096
    reusePrivateKey: true  # 핵심: SSL Pinning 호환 유지
  usages:
    - server auth
    - digital signature
    - key encipherment
```

### Certificate — 내부 서비스 (자동 로테이션, 핀 미적용)

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: payment-svc-cert
  namespace: payments
spec:
  secretName: payment-svc-tls
  issuerRef:
    name: aws-pca-payments
    kind: Issuer
  commonName: payment-svc.payments.svc.cluster.local
  dnsNames:
    - payment-svc.payments.svc.cluster.local
    - payment-svc.payments
  duration: 720h    # 30일
  renewBefore: 240h # 10일 전 갱신
  privateKey:
    algorithm: RSA
    size: 2048
    reusePrivateKey: false  # 내부 서비스: 매번 새 키 발급
  usages:
    - server auth
    - client auth  # mTLS 양방향
```

### Certificate — 클라이언트 인증서 (모바일 앱)

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: mobile-client-cert
  namespace: auth
spec:
  secretName: mobile-client-tls
  issuerRef:
    name: aws-pca-client
    kind: Issuer
  commonName: mobile-app-v2.fintech.com
  duration: 8760h
  renewBefore: 1440h # 60일 전 (앱 스토어 배포 리드타임 고려)
  privateKey:
    algorithm: RSA
    size: 2048
    reusePrivateKey: true
  usages:
    - client auth
    - digital signature
```

### Dual-Pin 전환 시 임시 Certificate (Backup Pin 운영)

```yaml
# 새 인증서 (병행 운영 기간 중)
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: api-fintech-cert-new
  namespace: istio-system
spec:
  secretName: api-fintech-tls-new
  issuerRef:
    name: aws-pca-api
    kind: ClusterIssuer
  commonName: api.fintech.com
  privateKey:
    reusePrivateKey: false  # 새 키 발급 (Backup Pin 활성화 후)
  # ... 나머지 동일
```

### IRSA — cert-manager가 PCA에 접근하기 위한 IAM 설정

```yaml
# cert-manager ServiceAccount에 IRSA 어노테이션
apiVersion: v1
kind: ServiceAccount
metadata:
  name: cert-manager
  namespace: cert-manager
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789:role/cert-manager-pca-role
```

```json
// IAM 정책 (최소 권한)
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "acm-pca:DescribeCertificateAuthority",
        "acm-pca:GetCertificate",
        "acm-pca:IssueCertificate"
      ],
      "Resource": [
        "arn:aws:acm-pca:ap-northeast-2:123456789:certificate-authority/infra-ca-id",
        "arn:aws:acm-pca:ap-northeast-2:123456789:certificate-authority/payments-ca-id"
      ]
    }
  ]
}
```

---

## 4. 트러블슈팅

### 문제 1: cert-manager가 PCA 인증서 발급 실패

```bash
# Certificate 상태 확인
kubectl describe certificate api-fintech-cert -n istio-system

# CertificateRequest 상태 확인 (실제 발급 요청)
kubectl get certificaterequest -n istio-system
kubectl describe certificaterequest api-fintech-cert-xxxx -n istio-system

# 일반적인 원인
# 1. IRSA 역할 신뢰 정책 미설정
# 2. PCA CA ARN 오타
# 3. PCA CA 상태가 ACTIVE가 아님
aws acm-pca describe-certificate-authority \
  --certificate-authority-arn arn:aws:acm-pca:...
```

### 문제 2: reusePrivateKey 설정 후에도 Public Key가 변경됨

```bash
# 현재 인증서 Public Key hash 확인 (SPKI)
kubectl get secret api-fintech-tls -n istio-system \
  -o jsonpath='{.data.tls\.crt}' | base64 -d | \
  openssl x509 -noout -pubkey | \
  openssl pkey -pubin -outform DER | \
  openssl dgst -sha256 -binary | base64

# 원인: Secret이 삭제되면 reusePrivateKey 효과 없음
# → Secret은 절대 삭제하지 말고 Certificate만 갱신
```

### 문제 3: 인증서 갱신 후 Istio가 새 인증서 반영 안 함

```bash
# Istio는 Secret 변경을 자동 감지하지만, 간혹 지연 발생
# Ingress Gateway Pod 재시작으로 강제 갱신
kubectl rollout restart deployment istio-ingressgateway -n istio-system

# Envoy가 로드한 인증서 만료일 확인
kubectl exec -n istio-system deploy/istio-ingressgateway -- \
  curl -s localhost:15000/certs | jq '.certificates[].cert_chain[0].days_until_expiration'
```

### 문제 4: PCA 비용 최적화

```
AWS PCA 비용 구조:
- CA 운영: $400/월 per CA (General Purpose)
- 인증서 발급: $0.75/인증서 (1,000개 이상 시 할인)

최적화 전략:
- 내부 서비스용: cert-manager 자체 CA Issuer 사용 (PCA 불필요)
- PCA는 외부 노출 및 클라이언트 인증서에만 사용
- Short-Lived CA 모드: $50/월 (단기 발급 전용, OCSP 없음)
```
