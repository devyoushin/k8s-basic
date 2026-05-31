# Falco — 런타임 보안 감지

## 1. 개요 및 비유

Falco는 Linux 시스템 콜을 실시간으로 감시해 컨테이너 탈출, 권한 상승, 비정상적인 파일/네트워크 접근 등을 즉시 탐지하는 CNCF 런타임 보안 도구.

💡 비유: 방화벽(NetworkPolicy)이 건물 출입구 통제라면, Falco는 건물 내부에 설치된 CCTV — 이미 들어온 사람이 금고를 열거나 비상구를 통해 탈출하려는 행동을 실시간으로 감지.

---

## 2. 핵심 설명

### 2.1 동작 원리

#### Falco 아키텍처

```
[컨테이너/Pod]
     │ 시스템 콜 (open, exec, connect, ...)
     ▼
[Linux 커널]
     │ eBPF probe 또는 커널 모듈
     ▼
[Falco Engine]  ← 규칙(Rules) 매칭
     │ 이벤트 감지
     ▼
[Falco Sidekick]  →  Slack / PagerDuty / SIEM / S3 알림
```

#### 감지 방식 비교

| 방식 | 메커니즘 | 특징 |
|------|---------|------|
| 커널 모듈 | `falco.ko` 로드 | 가장 성숙, 커널 업그레이드 시 재빌드 필요 |
| eBPF probe | CO-RE eBPF | 커널 버전 무관, 안정적 (권장) |
| Modern eBPF | 빌트인 eBPF | 외부 의존성 없음, Falco 0.35+ |

#### 주요 감지 시나리오 (금융권)

- **컨테이너 내 셸 실행**: `kubectl exec`나 악성코드가 `/bin/sh`, `/bin/bash` 실행
- **민감 파일 접근**: `/etc/shadow`, `/etc/kubernetes/pki/`, AWS 자격증명 파일 읽기
- **컨테이너 탈출 시도**: `nsenter`, `chroot` 실행, 호스트 namespace 접근
- **예상치 못한 네트워크 연결**: 컨테이너가 외부 C2 서버로 연결 시도
- **권한 상승**: `setuid` 바이너리 실행, `CAP_SYS_ADMIN` 획득 시도
- **크립토마이닝**: `cryptominer`, `xmrig` 등 프로세스 실행

---

### 2.2 YAML 적용 예시

#### Falco 설치 (Helm, eBPF 모드)

```bash
helm repo add falcosecurity https://falcosecurity.github.io/charts
helm repo update

helm install falco falcosecurity/falco \
  --namespace falco \
  --create-namespace \
  --set driver.kind=ebpf \
  --set falcosidekick.enabled=true \
  --set falcosidekick.config.slack.webhookurl=<SLACK_WEBHOOK_URL> \
  --set falcosidekick.config.slack.minimumpriority=warning \
  --set collectors.kubernetes.enabled=true
```

#### Falco 커스텀 규칙 — 금융권 특화

```yaml
# ConfigMap으로 커스텀 규칙 주입
apiVersion: v1
kind: ConfigMap
metadata:
  name: falco-custom-rules
  namespace: falco
data:
  custom-rules.yaml: |
    # 규칙 1: payments 네임스페이스 컨테이너 내 셸 실행 감지
    - rule: Shell Executed in Payment Container
      desc: payments 네임스페이스 Pod에서 셸이 실행됨 (침해 의심)
      condition: >
        spawned_process
        and shell_procs
        and k8s.ns.name = "payments"
      output: >
        셸 실행 감지 (user=%user.name cmd=%proc.cmdline
        pod=%k8s.pod.name ns=%k8s.ns.name image=%container.image)
      priority: CRITICAL
      tags: [payment, shell, incident]

    # 규칙 2: Kubernetes 인증서 디렉토리 접근
    - rule: K8s PKI Directory Access
      desc: /etc/kubernetes/pki 접근 감지
      condition: >
        open_read
        and fd.name startswith /etc/kubernetes/pki
        and not proc.name in (kube-apiserver, etcd)
      output: >
        PKI 디렉토리 접근 (user=%user.name proc=%proc.name
        file=%fd.name pod=%k8s.pod.name)
      priority: CRITICAL
      tags: [k8s, pki, credential-access]

    # 규칙 3: AWS 자격증명 파일 접근
    - rule: AWS Credentials Access
      desc: AWS 자격증명 파일 접근 감지
      condition: >
        open_read
        and (fd.name = "/root/.aws/credentials"
             or fd.name = "/home/.aws/credentials"
             or fd.name startswith "/var/run/secrets/")
        and not proc.name in (aws-cli, boto3)
      output: >
        AWS 자격증명 접근 (proc=%proc.name file=%fd.name
        pod=%k8s.pod.name ns=%k8s.ns.name)
      priority: WARNING
      tags: [aws, credential-access]

    # 규칙 4: 컨테이너에서 예상치 못한 외부 연결
    - rule: Unexpected Outbound Connection from Payment Pod
      desc: payments Pod에서 허용되지 않은 외부 IP로 연결
      condition: >
        outbound
        and k8s.ns.name = "payments"
        and not fd.sip in (payment_allowed_ips)
        and not fd.sport in (payment_allowed_ports)
      output: >
        비허가 외부 연결 (src=%fd.cip dst=%fd.sip:%fd.sport
        proc=%proc.name pod=%k8s.pod.name)
      priority: WARNING
      tags: [payment, network, exfiltration]

    # 매크로: 허용된 결제 서비스 IP 목록
    - macro: payment_allowed_ips
      condition: >
        fd.sip in ("10.0.0.0/8", "172.16.0.0/12")

    - macro: payment_allowed_ports
      condition: fd.sport in (443, 6380, 5432)

    # 규칙 5: 크립토마이닝 프로세스 감지
    - rule: Cryptominer Execution
      desc: 알려진 크립토마이닝 프로세스 실행 감지
      condition: >
        spawned_process
        and proc.name in (xmrig, minerd, cgminer, bfgminer, cpuminer)
      output: >
        크립토마이너 실행 (proc=%proc.name cmdline=%proc.cmdline
        pod=%k8s.pod.name ns=%k8s.ns.name)
      priority: CRITICAL
      tags: [cryptomining, malware]
```

#### Falco + Falcosidekick — Slack/PagerDuty 알림 설정

```yaml
# Helm values.yaml
falcosidekick:
  enabled: true
  config:
    slack:
      webhookurl: "<SLACK_WEBHOOK_URL>"
      minimumpriority: "warning"
      messageformat: |
        *[Falco Alert]* `{priority}` - {rule}
        > {output}
    pagerduty:
      routingKey: "<PAGERDUTY_ROUTING_KEY>"
      minimumpriority: "critical"
    # S3에 이벤트 아카이빙 (감사 로그)
    aws:
      s3:
        bucket: "fintech-falco-events"
        prefix: "falco/"
        minimumpriority: "notice"
```

#### Falco DaemonSet 보안 컨텍스트

```yaml
# Falco는 privileged 필요 (커널 접근)
# PSA 정책에서 falco 네임스페이스 제외 필요
apiVersion: v1
kind: Namespace
metadata:
  name: falco
  labels:
    pod-security.kubernetes.io/enforce: privileged
    pod-security.kubernetes.io/enforce-version: latest
```

---

### 2.3 Best Practice

- **기본 규칙 + 커스텀 규칙 분리 관리**: `falco_rules.yaml`(기본)은 업스트림 유지, `falco_rules.local.yaml`에 커스텀 규칙 추가 — 업그레이드 시 충돌 방지
- **우선순위 기반 알림 분리**: `CRITICAL` → PagerDuty 즉시 호출, `WARNING` → Slack 채널, `NOTICE` → S3 아카이빙만
- **허용 목록(whitelist) 먼저 구축**: 초기 배포 시 과탐(False Positive) 많음 → `audit` 모드로 1~2주 운영하며 정상 패턴 파악 후 규칙 정교화
- **Falco + OPA/Kyverno 조합**: Falco는 런타임 감지, OPA는 배포 전 예방 — 두 레이어를 함께 운영
- **eBPF 모드 사용**: 커널 모듈보다 안정적이고 노드 재부팅 없이 업그레이드 가능

---

## 3. 트러블슈팅

### 3.1 주요 이슈

#### 과탐(False Positive)으로 알림 폭주

**증상**: Slack에 동일한 알림이 수백 건씩 발생

**원인**: 정상 운영 중인 프로세스가 규칙에 매칭됨 (예: 배포 시 셸 실행)

**해결 방법**:
```yaml
# 특정 프로세스/이미지를 규칙에서 제외
- rule: Shell Executed in Payment Container
  condition: >
    spawned_process
    and shell_procs
    and k8s.ns.name = "payments"
    and not proc.pname in (kubectl, helm, argocd)  # 배포 도구 제외
    and not container.image.repository = "bitnami/kubectl"
```

```bash
# 어떤 이벤트가 가장 많이 발생하는지 확인
kubectl logs -n falco ds/falco | grep "rule=" | \
  sed 's/.*rule="\([^"]*\)".*/\1/' | sort | uniq -c | sort -rn | head -20
```

#### Falco Pod가 노드에서 시작 안 됨 (eBPF probe 로드 실패)

**증상**: Falco DaemonSet Pod가 `Error` 상태, 로그에 `probe load failed`

**원인**: 커널 헤더 미설치 또는 eBPF 미지원 커널

**해결 방법**:
```bash
# 커널 버전 확인 (eBPF 권장: 5.8+)
uname -r

# 노드에서 eBPF 지원 확인
ls /sys/kernel/btf/vmlinux

# Modern eBPF 모드로 전환 (커널 헤더 불필요)
helm upgrade falco falcosecurity/falco -n falco \
  --set driver.kind=modern_ebpf
```

---

### 3.2 자주 발생하는 문제

#### Falco가 Kubernetes 메타데이터(Pod명, 네임스페이스) 출력 안 함

**증상**: 알림에 `k8s.pod.name` 등이 `<NA>`로 표시

**원인**: Falco가 Kubernetes API에 접근하지 못함 (collectors 비활성화)

**해결 방법**:
```bash
helm upgrade falco falcosecurity/falco -n falco \
  --set collectors.kubernetes.enabled=true

# RBAC 권한 확인
kubectl get clusterrolebinding -l app.kubernetes.io/name=falco
```

---

## 4. 모니터링 및 확인

```bash
# Falco 이벤트 실시간 확인
kubectl logs -n falco ds/falco -f | grep -E "Warning|Critical|Error"

# Falco 규칙 로딩 확인
kubectl logs -n falco ds/falco | grep -i "rule\|load"

# 적용된 규칙 목록 확인
kubectl exec -n falco ds/falco -- falco --list | head -50

# Falcosidekick 상태
kubectl logs -n falco deploy/falco-falcosidekick | tail -20

# 특정 Pod의 Falco 이벤트 필터링
kubectl logs -n falco ds/falco | grep "pod=<POD_NAME>"

# 이벤트 통계 (규칙별 발생 횟수)
kubectl logs -n falco ds/falco | \
  grep "Notice\|Warning\|Critical" | \
  grep -oP 'rule="\K[^"]+' | \
  sort | uniq -c | sort -rn
```

---

## 5. TIP

- **Falco Rules Hub**: `https://thomas.labarussias.fr/falco-rules-hub/` — 커뮤니티가 작성한 규칙 모음으로 금융, 컨테이너 탈출, 랜섬웨어 등 시나리오별 규칙 검색 가능
- **Tetragon 비교**: Cilium Tetragon은 eBPF 기반으로 Falco보다 낮은 오버헤드, 프로세스 킬(Kill) 액션 지원 — 탐지만이 아닌 즉각 차단이 필요한 환경에서 유리
- **감사 로그 연계**: Falco 이벤트를 S3 → AWS Athena로 연계하면 장기 보관 및 SQL 쿼리 기반 포렌식 분석 가능
- **PodSecurityAdmission + Falco**: PSA로 배포 전 예방, Falco로 런타임 감지 — "예방 + 감지" 두 레이어 운영이 금융권 컴플라이언스(PCI-DSS, ISO 27001) 요구사항 충족에 유리
