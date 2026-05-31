# SSL Pinning — 클라이언트 인증서 핀 전략

## 1. 개요 및 비유

SSL Pinning은 클라이언트(모바일 앱 등)가 서버 인증서를 신뢰하는 방식을 "CA 체인 신뢰"에서 "특정 키/인증서 직접 신뢰"로 좁히는 보안 기법입니다.
비유: 일반 TLS가 "공인된 모든 신분증 발급 기관을 믿는 것"이라면, SSL Pinning은 "이 은행은 오직 우리 회사 발급 신분증만 받겠다"는 정책입니다.

---

## 2. 핵심 설명

### Pinning 대상 비교

| 핀 대상 | 설명 | 장점 | 단점 |
|---------|------|------|------|
| 인증서 전체 | DER 인코딩 해시 핀 | 가장 강력 | 인증서 갱신마다 앱 업데이트 필수 |
| **Public Key (SPKI hash)** | SubjectPublicKeyInfo 해시 핀 | 인증서 갱신해도 키 유지 시 핀 유지 | 키 로테이션 시 앱 업데이트 필요 |
| Intermediate CA | CA 공개키 핀 | 유연성 높음 | 핀 범위 넓어 보안 약화 |

**금융권 권장: Public Key (SPKI hash) 핀 + Backup Pin 2개 이상**

### SPKI Hash 계산

```bash
# 서버 인증서에서 SPKI hash 추출
openssl s_client -connect api.fintech.com:443 2>/dev/null | \
  openssl x509 -pubkey -noout | \
  openssl pkey -pubin -outform DER | \
  openssl dgst -sha256 -binary | \
  base64

# 로컬 인증서 파일에서 추출
openssl x509 -in api.fintech.com.crt -pubkey -noout | \
  openssl pkey -pubin -outform DER | \
  openssl dgst -sha256 -binary | \
  base64
```

### Backup Pin 전략

핀이 걸린 상태에서 인증서가 만료되거나 침해될 경우를 대비해 Backup Pin이 필수입니다.

```
Primary Pin: 현재 운영 인증서의 SPKI hash
Backup Pin 1: 다음 인증서의 SPKI hash (사전 준비된 키 쌍)
Backup Pin 2: Intermediate CA의 SPKI hash (최후 보루)
```

**Backup Pin 운영 원칙:**
- 항상 최소 2개의 유효한 Pin 유지
- Primary가 만료되면 Backup 1이 Primary로 승격
- 새로운 Backup Pin을 앱 업데이트 전에 서버 측에서 먼저 활성화

### Dual-Pin 전환 절차 (Zero-Downtime)

```
[단계 1] 새 키 쌍 생성 → 새 인증서 발급 (cert-manager)
[단계 2] 신규 SPKI hash를 앱에 Backup Pin으로 추가 → 앱 배포
[단계 3] 앱 배포 완료 확인 (구버전 앱 비율 모니터링)
[단계 4] 서버 인증서를 신규 인증서로 교체 (새 Primary)
[단계 5] 구 인증서 SPKI hash를 앱에서 Backup Pin으로 유지 (한 달)
[단계 6] 구 Pin 제거 → 앱 업데이트
```

### Certificate Transparency (CT) 모니터링

자사 도메인에 대한 의도치 않은 인증서 발급을 조기 감지합니다.

- **목적**: 악의적인 CA가 `api.fintech.com`에 대한 인증서를 발급해도 즉시 탐지
- **도구**: crt.sh API, Google Certificate Transparency, Facebook CT Monitor
- **대응**: CT 로그 모니터링 → 미승인 인증서 발견 시 즉시 해당 CA에 폐기 요청

### OCSP Stapling

클라이언트가 OCSP 서버에 직접 폐기 확인 요청하지 않고, 서버가 OCSP 응답을 미리 첨부(Staple)합니다.

**이점:**
- 클라이언트 프라이버시 보호 (CA에 접속 이력 노출 없음)
- 검증 속도 향상 (추가 네트워크 왕복 제거)
- AWS ALB/NLB: 자동 OCSP Stapling 지원

---

## 3. YAML 적용 예시

### Istio Gateway — TLS 1.3 강제 + Cipher Suite 제한

```yaml
# SSL Pinning 대상 엔드포인트: 최강 TLS 설정
apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: public-api-gateway
  namespace: istio-system
spec:
  selector:
    istio: ingressgateway
  servers:
    - port:
        number: 443
        name: https
        protocol: HTTPS
      tls:
        mode: SIMPLE
        credentialName: api-fintech-tls  # cert-manager 관리 인증서
        minProtocolVersion: TLSV1_3
        cipherSuites:
          # FIPS 140-2 준수 Cipher Suite
          - TLS_AES_256_GCM_SHA384
          - TLS_CHACHA20_POLY1305_SHA256
      hosts:
        - "api.fintech.com"
```

### cert-manager — SSL Pinning 호환 인증서 (Key Reuse)

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: api-fintech-pinned
  namespace: istio-system
spec:
  secretName: api-fintech-tls
  issuerRef:
    name: aws-pca-api
    kind: ClusterIssuer
  commonName: api.fintech.com
  dnsNames:
    - api.fintech.com
  duration: 8760h    # 1년 (장기 유효로 핀 안정성 확보)
  renewBefore: 720h  # 30일 전 갱신 (앱 업데이트 준비 시간)
  privateKey:
    algorithm: RSA
    size: 4096
    reusePrivateKey: true  # 핵심: 갱신해도 Public Key 불변
```

### CT 모니터링 — CronJob으로 자동화

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: ct-log-monitor
  namespace: security
spec:
  schedule: "0 */6 * * *"  # 6시간마다 확인
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: ct-monitor
              image: curlimages/curl:latest
              command:
                - /bin/sh
                - -c
                - |
                  # crt.sh API로 자사 도메인 인증서 조회
                  CERTS=$(curl -s "https://crt.sh/?q=%.fintech.com&output=json")
                  COUNT=$(echo $CERTS | python3 -c "import sys,json; data=json.load(sys.stdin); print(len(data))")
                  echo "발견된 인증서 수: $COUNT"
                  # 미승인 발급자 확인 후 Slack 알림
                  echo $CERTS | python3 -c "
                  import sys, json
                  data = json.load(sys.stdin)
                  for cert in data:
                    if 'Amazon' not in cert.get('issuer_name','') and 'Company' not in cert.get('issuer_name',''):
                      print('미승인 인증서 감지:', cert)
                  "
          restartPolicy: OnFailure
```

### iOS SSL Pinning 구현 예시 (참고용 Swift)

```swift
// URLSession Delegate로 SPKI hash 검증
class PinnedURLSessionDelegate: NSObject, URLSessionDelegate {
    // 현재 Pin + Backup Pin
    let pinnedSPKIHashes: Set<String> = [
        "sha256/AAAA...현재 SPKI hash...==",   // Primary Pin
        "sha256/BBBB...백업 SPKI hash...==",   // Backup Pin 1
        "sha256/CCCC...중간 CA hash...==",     // Backup Pin 2 (Intermediate CA)
    ]

    func urlSession(
        _ session: URLSession,
        didReceive challenge: URLAuthenticationChallenge,
        completionHandler: @escaping (URLSession.AuthChallengeDisposition, URLCredential?) -> Void
    ) {
        guard challenge.protectionSpace.authenticationMethod == NSURLAuthenticationMethodServerTrust,
              let serverTrust = challenge.protectionSpace.serverTrust,
              let certificate = SecTrustGetCertificateAtIndex(serverTrust, 0) else {
            completionHandler(.cancelAuthenticationChallenge, nil)
            return
        }

        // 서버 인증서의 Public Key 추출 및 SPKI hash 계산
        let serverSPKIHash = calculateSPKIHash(from: certificate)

        if pinnedSPKIHashes.contains(serverSPKIHash) {
            completionHandler(.useCredential, URLCredential(trust: serverTrust))
        } else {
            // 핀 불일치 → 연결 차단 + 보안팀 알림
            reportPinMismatch(serverSPKIHash)
            completionHandler(.cancelAuthenticationChallenge, nil)
        }
    }
}
```

### Android SSL Pinning (OkHttp)

```kotlin
// build.gradle.kts
// implementation("com.squareup.okhttp3:okhttp:4.12.0")

val client = OkHttpClient.Builder()
    .certificatePinner(
        CertificatePinner.Builder()
            // Primary Pin + Backup Pins
            .add("api.fintech.com", "sha256/AAAA...현재 SPKI hash...==")
            .add("api.fintech.com", "sha256/BBBB...백업 SPKI hash...==")
            .add("api.fintech.com", "sha256/CCCC...중간 CA hash...==")
            .build()
    )
    .build()
```

### Network Policy — Egress 제한으로 핀 우회 방지

```yaml
# Pod에서 허용된 외부 도메인 외 통신 차단
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: payment-svc-egress
  namespace: payments
spec:
  podSelector:
    matchLabels:
      app: payment-svc
  policyTypes:
    - Egress
  egress:
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: payments
    - to:
        - ipBlock:
            cidr: 10.0.0.0/8  # 내부 VPC만 허용
      ports:
        - protocol: TCP
          port: 6380  # Redis TLS
        - protocol: TCP
          port: 5432  # RDS PostgreSQL
```

---

## 4. 트러블슈팅

### 문제 1: 인증서 갱신 후 모바일 앱 통신 불가 (핀 불일치)

```
증상: 앱에서 SSLPeerUnverifiedException / NSURLErrorServerCertificateUntrusted

원인 진단:
1. cert-manager가 reusePrivateKey: false로 갱신 → Public Key 변경
2. 이전 Secret 삭제 후 재생성 → reusePrivateKey 효과 없음

긴급 복구:
1. 구 인증서 백업이 있다면 즉시 롤백
   kubectl create secret tls api-fintech-tls --cert=old.crt --key=old.key -n istio-system --dry-run=client -o yaml | kubectl apply -f -

2. Backup Pin이 설정된 경우: 구 Intermediate CA pin으로 임시 연결
   → 앱 강제 업데이트 팝업 표시 (MDM/Firebase Remote Config 활용)

3. 장기 대응: cert-manager Certificate에 reusePrivateKey: true 설정
```

### 문제 2: 개발/테스트 환경에서 핀 우회 필요

```
방법 1: 환경별 핀 분리 (권장)
  - 프로덕션 Pin: api.fintech.com (하드코딩)
  - 개발 Pin: api-dev.fintech.com (별도 핀 or 핀 비활성화)
  - 빌드 플래그로 환경 분리

방법 2: 프록시 도구용 임시 비활성화
  - Charles Proxy / mitmproxy 루트 CA를 Backup Pin으로 임시 등록
  - 절대 프로덕션 빌드에 포함 금지
  - CI/CD에서 프로덕션 빌드 시 핀 설정 검증 스텝 추가
```

### 문제 3: CT 로그에서 미승인 인증서 발견

```
즉시 조치:
1. 해당 CA에 인증서 폐기 요청 (Revocation)
2. 브라우저 벤더(Chrome, Firefox)에 CA 불신 보고 검토
3. 자사 도메인 CAA 레코드 강화:

  # Route53 CAA 레코드 설정
  fintech.com. CAA 0 issue "amazon.com"
  fintech.com. CAA 0 issuewild ";"  # 와일드카드 금지
  fintech.com. CAA 0 iodef "mailto:security@fintech.com"

4. 사용자에게 공지 (영향 범위 파악 후)
```

### 문제 4: OkHttp CertificatePinner 핀 해시값 확인

```bash
# 서버에서 직접 SPKI hash 확인 (OkHttp 형식)
echo | openssl s_client -connect api.fintech.com:443 2>/dev/null | \
  openssl x509 -pubkey -noout | \
  openssl pkey -pubin -outform DER | \
  openssl dgst -sha256 -binary | \
  base64
# 출력값을 "sha256/..." 형태로 OkHttp에 설정
```
