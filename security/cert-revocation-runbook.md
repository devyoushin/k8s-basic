# 인증서 긴급 폐기 대응 런북

## 1. 개요 및 비유

인증서 Private Key가 침해(Compromise)되거나 오발급된 경우, 신속하게 폐기하고 새 인증서로 전환하는 긴급 대응 절차입니다.
비유: 신용카드 분실 시 "즉시 정지 → 새 카드 발급 → 자동결제 정보 업데이트"처럼, 단계적이고 신속한 전환이 핵심입니다.

---

## 2. 핵심 설명

### 긴급도 분류 (Severity)

| 등급 | 상황 | 대응 시간 |
|------|------|----------|
| P0 (Critical) | Private Key 유출 확인 / 미승인 인증서로 트래픽 탈취 의심 | 즉시 (30분 이내) |
| P1 (High) | CT 로그에서 미승인 인증서 발견 / 내부 감사에서 키 접근 이상 탐지 | 2시간 이내 |
| P2 (Medium) | 예정된 키 로테이션 (SSL Pinning 전환) / 알고리즘 취약점 대응 | 계획된 일정 내 |

### 폐기 전파 경로

```
[인증서 폐기 요청]
        │
        ├── AWS PCA: 즉시 CRL 업데이트 (수 초)
        │           OCSP 응답 갱신 (수 분)
        │
        ├── Istio mesh: cert-manager → Secret 업데이트
        │              Istiod → Envoy sidecar 인증서 교체
        │
        ├── ALB/NLB: ACM 인증서 교체 (수 분, 무중단)
        │
        └── 핀된 클라이언트: 앱 업데이트 필요 (별도 절차)
```

### SSL Pinning 인증서 긴급 폐기의 특수성

일반 인증서 폐기와 달리, SSL Pinning 환경에서는:
1. 서버 인증서 교체만으로는 부족 — 클라이언트 앱도 새 Pin으로 업데이트 필요
2. Backup Pin이 없으면 전체 앱 사용자가 통신 불가 상태에 빠질 수 있음
3. **사전 Backup Pin 준비 여부가 대응 속도를 결정함**

---

## 3. 긴급 대응 절차

### Phase 0: 초기 탐지 및 판단 (0~15분)

```bash
# 1. 어떤 인증서가 침해되었는지 확인
openssl x509 -in <compromised.crt> -noout -text | grep -E "Subject:|Issuer:|Not After"

# 2. 해당 인증서를 사용 중인 서비스 확인
kubectl get secrets -A -o json | \
  python3 -c "
import sys, json, base64
data = json.load(sys.stdin)
for item in data['items']:
    if 'tls.crt' in item.get('data', {}):
        print(item['metadata']['namespace'], item['metadata']['name'])
"

# 3. 현재 인증서 지문 확인 (서버에서 실제 서빙 중인 인증서)
echo | openssl s_client -connect api.fintech.com:443 2>/dev/null | \
  openssl x509 -noout -fingerprint -sha256

# 4. 판단: Backup Pin 준비 여부 확인
# → 있으면: Phase 1 즉시 진행
# → 없으면: Phase 1 + 앱팀 긴급 연락 병행
```

### Phase 1: 즉시 폐기 (15~30분)

```bash
# AWS PCA에서 인증서 즉시 폐기
aws acm-pca revoke-certificate \
  --certificate-authority-arn arn:aws:acm-pca:ap-northeast-2:123456789:certificate-authority/api-ca-id \
  --certificate-serial <SERIAL_NUMBER> \
  --revocation-reason KEY_COMPROMISE

# 시리얼 번호 확인 방법
openssl x509 -in compromised.crt -noout -serial

# CRL 업데이트 확인
aws acm-pca describe-certificate-authority \
  --certificate-authority-arn arn:aws:acm-pca:... \
  --query 'CertificateAuthority.RevocationConfiguration'
```

### Phase 2: 새 인증서 발급 및 교체 (30분~1시간)

```bash
# 방법 1: cert-manager 강제 갱신 (reusePrivateKey: false 임시 설정)
kubectl patch certificate api-fintech-pinned -n istio-system \
  --type='json' \
  -p='[{"op":"replace","path":"/spec/privateKey/reusePrivateKey","value":false}]'

# cert-manager 강제 갱신 트리거
kubectl annotate certificate api-fintech-pinned -n istio-system \
  cert-manager.io/issue-temporary-certificate="true" \
  cert-manager.io/issuer-name="aws-pca-api" \
  --overwrite

# 또는: CertificateRequest 직접 생성
kubectl cmctl renew api-fintech-pinned -n istio-system

# 방법 2: 사전 준비된 비상용 키/인증서로 즉시 교체
kubectl create secret tls api-fintech-tls \
  --cert=emergency-cert.pem \
  --key=emergency-key.pem \
  -n istio-system \
  --dry-run=client -o yaml | kubectl apply -f -

# Istio Ingress Gateway 재시작 (새 인증서 즉시 반영)
kubectl rollout restart deployment istio-ingressgateway -n istio-system
kubectl rollout status deployment istio-ingressgateway -n istio-system

# ACM 인증서 교체 (ALB 사용 시)
aws acm import-certificate \
  --certificate-arn arn:aws:acm:... \
  --certificate file://new-cert.pem \
  --private-key file://new-key.pem \
  --certificate-chain file://chain.pem
```

### Phase 3: SSL Pinning 앱 대응 (병행 진행)

```bash
# Backup Pin이 있는 경우:
# 새 인증서의 SPKI hash를 Backup Pin으로 사전 등록한 앱이 배포된 경우
# → 서버 인증서 교체만으로 통신 복구 가능

# 새 인증서 SPKI hash 계산
openssl x509 -in new-cert.pem -pubkey -noout | \
  openssl pkey -pubin -outform DER | \
  openssl dgst -sha256 -binary | base64
# 이 값이 앱의 Backup Pin과 일치해야 함

# Backup Pin이 없는 경우 (최악의 시나리오):
# 1. Firebase Remote Config로 핀 검증 비활성화 (긴급 킬스위치)
# 2. 앱스토어 긴급 심사 요청 (iOS: Expedite Review 신청)
# 3. MDM으로 기업 단말에 강제 업데이트 배포
# 4. 사용자에게 강제 업데이트 안내 팝업 (앱 최소 버전 강제)
```

### Phase 4: JWT/Session 무효화 (침해 범위에 따라)

```bash
# 침해된 키로 서명된 JWT가 있는 경우 전수 무효화

# Redis에서 세션 전체 삭제 (키 침해 확인 시점 이전 세션)
kubectl exec -n payments deploy/redis-master -- \
  redis-cli -a <password> \
  EVAL "return redis.call('del', unpack(redis.call('keys', 'session:*')))" 0

# JWT 블랙리스트에 침해 키 ID(kid) 추가
kubectl exec -n auth deploy/auth-svc -- \
  curl -X POST localhost:8080/admin/blacklist-key \
  -H "Content-Type: application/json" \
  -d '{"kid": "compromised-key-id", "revoked_at": "2026-05-10T00:00:00Z"}'

# 영향받은 사용자에게 재로그인 강제
```

### Phase 5: 사후 검증 (1~2시간)

```bash
# 새 인증서가 정상 서빙되는지 확인
echo | openssl s_client -connect api.fintech.com:443 2>/dev/null | \
  openssl x509 -noout -dates -fingerprint

# Istio Envoy에서 새 인증서 반영 확인
istioctl proxy-config secret <pod-name> -n payments | head -20

# OCSP 폐기 상태 확인
openssl ocsp \
  -issuer intermediate-ca.pem \
  -cert compromised.pem \
  -url http://ocsp.amazonaws.com \
  -text

# 구 인증서가 더 이상 사용되지 않는지 확인
# (로드밸런서 로그, Istio 액세스 로그 모니터링)
kubectl logs -n istio-system deploy/istio-ingressgateway | \
  grep -i "cert\|tls" | tail -50
```

---

## 4. 사전 준비 체크리스트

### 비상용 인증서 사전 준비

```bash
# 정기적으로 비상용 키 쌍 생성 및 안전한 저장소에 보관
# (AWS Secrets Manager 또는 HashiCorp Vault)

# 비상용 키 쌍 생성
openssl genrsa -out emergency-key.pem 4096
openssl req -new -key emergency-key.pem \
  -out emergency-csr.pem \
  -subj "/CN=api.fintech.com/O=Fintech Corp"

# PCA에서 인증서 발급 (비상용, 유효기간 1년)
aws acm-pca issue-certificate \
  --certificate-authority-arn arn:aws:acm-pca:... \
  --csr file://emergency-csr.pem \
  --signing-algorithm SHA512WITHRSA \
  --validity Value=365,Type=DAYS

# SPKI hash 추출 → 앱의 Backup Pin으로 사전 등록
openssl req -in emergency-csr.pem -pubkey -noout | \
  openssl pkey -pubin -outform DER | \
  openssl dgst -sha256 -binary | base64

# AWS Secrets Manager에 안전하게 저장
aws secretsmanager create-secret \
  --name "fintech/emergency-cert/api" \
  --secret-string file://emergency-key.pem
```

### 분기별 훈련 체크리스트

```
□ 비상용 인증서 유효기간 확인 (만료 전 갱신)
□ 비상용 인증서 SPKI hash가 앱 Backup Pin에 등록되어 있는지 확인
□ 폐기 절차 시뮬레이션 (스테이징 환경)
□ 앱팀과 긴급 연락망 최신화
□ MDM 강제 업데이트 절차 테스트
□ Firebase Remote Config 킬스위치 동작 확인
□ CT 로그 모니터링 알림 설정 확인
□ AWS PCA CRL 배포 설정 확인
```

---

## 5. 트러블슈팅

### 문제 1: PCA 인증서 폐기 후 OCSP 반영 지연

```
AWS PCA OCSP 갱신 지연 시간: 최대 60분

임시 대응:
- 구 인증서를 즉시 서버에서 제거 (새 인증서로 교체)
- CRL은 즉시 반영되므로 CRL 기반 검증으로 전환

CRL 다운로드 URL 확인:
openssl x509 -in cert.pem -noout -text | grep -A 3 "CRL Distribution"
```

### 문제 2: cert-manager Secret 교체 후 Istio가 구 인증서 계속 사용

```bash
# Envoy가 로드 중인 인증서 확인
kubectl exec -n istio-system deploy/istio-ingressgateway -- \
  curl -s localhost:15000/certs | jq '.'

# Istio-proxy는 파일 시스템 변경을 감지하지만,
# 종종 Pod 재시작이 더 빠름
kubectl rollout restart deployment istio-ingressgateway -n istio-system

# SDS (Secret Discovery Service) 갱신 강제
kubectl exec <pod-name> -n payments -c istio-proxy -- \
  pilot-agent request GET /config_dump | grep -i "cert_chain"
```

### 문제 3: 인증서 교체 중 TLS Handshake 실패 증가

```bash
# ALB 액세스 로그에서 SSL 오류 카운트 모니터링
aws logs filter-log-events \
  --log-group-name /aws/elasticloadbalancing/api-alb \
  --filter-pattern "\"ssl_error\"" \
  --start-time $(date -d '10 minutes ago' +%s000)

# Istio 메트릭에서 TLS 오류 확인
kubectl exec -n payments deploy/payment-svc -c istio-proxy -- \
  curl -s localhost:15090/stats | grep ssl_handshake_error

# 롤링 업데이트 중 구/신 인증서 병행 서빙 (Blue/Green)
# → Istio Gateway의 credentialName을 단계적으로 전환
```
