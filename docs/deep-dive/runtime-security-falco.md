## 1. 개요 및 비유

런타임 보안은 컨테이너가 **실제 실행 중**에 의심스러운 행동을 탐지합니다. 이미지 스캔이 "입장 전 검사"라면, 런타임 보안은 "건물 내부 CCTV"입니다.

💡 **비유하자면 은행의 이상 거래 탐지 시스템(FDS)과 같습니다.**
정상 패턴에서 벗어난 행동(새벽에 갑자기 /etc/passwd 읽기, curl로 외부 접속 등)을 실시간으로 감지하고 알림을 보냅니다.

---

## 2. Falco 아키텍처

### 2.1 감시 레이어

```
컨테이너 프로세스가 syscall 호출
        │
        ▼
Linux 커널
  ├── [eBPF probe / kernel module]  ← Falco 감시 포인트
  │    모든 syscall 이벤트 캡처
  └── Falco 드라이버 (데이터 수집)
        │
        ▼
Falco 엔진 (유저스페이스)
  - 룰 매칭 (YAML 정의 룰셋)
  - 이벤트 필터링 & 평가
        │
        ▼
알림 출력:
  - stdout / syslog
  - Webhook (Slack, PagerDuty)
  - gRPC stream (Falcosidekick)
  - Kubernetes Audit Events
```

### 2.2 eBPF vs 커널 모듈

```
커널 모듈 드라이버:
  - 커널 버전에 직접 삽입
  - 높은 성능
  - 커널 업그레이드 시 재컴파일 필요
  - 커널 패닉 위험

eBPF 드라이버 (권장):
  - 안전한 샌드박스 실행 (커널 패닉 없음)
  - 커널 5.8+ 필요 (CO-RE로 이식성 확보)
  - 프로덕션 환경 권장

Modern eBPF (Falco 0.35+):
  - 별도 모듈 설치 없이 BTF 기반 동작
  - 가장 간단한 설치
```

---

## 3. Falco 설치 및 기본 설정

```bash
# Helm으로 설치
helm repo add falcosecurity https://falcosecurity.github.io/charts
helm install falco falcosecurity/falco \
  --namespace falco \
  --create-namespace \
  --set driver.kind=ebpf \          # eBPF 드라이버 사용
  --set falcosidekick.enabled=true \ # 알림 라우팅 활성화
  --set falcosidekick.webui.enabled=true  # 웹 UI

# DaemonSet으로 배포됨 (모든 노드)
kubectl get pods -n falco
```

```yaml
# falco ConfigMap 핵심 설정
json_output: true           # JSON 형식 출력 (파싱 편의)
log_stderr: true
log_level: info
priority: WARNING            # WARNING 이상만 출력 (DEBUG/INFO 필터)

outputs:
  rate: 1
  max_burst: 1000

# 출력 채널
stdout_output:
  enabled: true

webhook_output:
  enabled: true
  url: http://falcosidekick:2801   # Falcosidekick으로 전달
```

---

## 4. Falco 룰 심층 분석

### 4.1 룰 구조

```yaml
# Macro: 재사용 가능한 조건
- macro: spawned_process
  condition: evt.type = execve and evt.dir = <

- macro: container
  condition: container.id != host

# List: 허용/금지 목록
- list: shell_binaries
  items: [bash, sh, zsh, fish, dash]

- list: trusted_namespaces
  items: [kube-system, monitoring, falco]

# Rule: 탐지 룰
- rule: Terminal Shell in Container
  desc: 컨테이너 내부에서 인터랙티브 쉘 실행 감지
  condition: >
    spawned_process
    and container
    and shell_procs
    and proc.tty != 0            # 터미널(TTY)에서 실행
    and not trusted_containers
  output: >
    A shell was spawned in a container with an attached terminal
    (user=%user.name user_loginuid=%user.loginuid
     k8s.pod=%k8s.pod.name container=%container.id
     shell=%proc.name parent=%proc.pname
     cmdline=%proc.cmdline image=%container.image.repository)
  priority: NOTICE
  tags: [container, shell, mitre_execution]
```

### 4.2 주요 기본 룰 예시

```yaml
# 1. 컨테이너에서 민감한 파일 읽기
- rule: Read sensitive file untrusted
  condition: >
    open_read
    and sensitive_files           # /etc/passwd, /etc/shadow 등
    and not trusted_programs
    and container
  priority: WARNING

# 2. 컨테이너 탈출 시도 (privileged 실행 감지는 Admission에서)
- rule: Container Drift Detected (chmod)
  condition: >
    container
    and chmod
    and (evt.arg.mode contains "S_ISUID" or    # setuid 비트
         evt.arg.mode contains "S_ISGID")
  priority: ERROR

# 3. 예상치 못한 아웃바운드 네트워크 연결
- rule: Unexpected outbound connection destination
  condition: >
    outbound
    and container
    and not fd.sport in (80, 443, 8080, 8443)  # 허용 포트 외
    and not trusted_images
  priority: WARNING

# 4. 크립토마이닝 탐지
- rule: Detect crypto miners using the Stratum protocol
  condition: >
    spawned_process
    and container
    and (proc.cmdline contains "stratum" or
         proc.cmdline contains "xmrig" or
         proc.cmdline contains "minergate")
  priority: CRITICAL

# 5. 패키지 관리자 실행 (컨테이너 내부 변조)
- rule: Package Management Tools in Container
  condition: >
    spawned_process
    and container
    and package_mgmt_procs        # apt, yum, pip 등
    and not user_known_package_manager_in_container
  priority: ERROR
```

### 4.3 커스텀 룰 작성 (애플리케이션 특화)

```yaml
# 특정 앱(nginx)이 비정상적 파일 쓰기 시 탐지
- rule: Nginx writes unexpected file
  desc: Nginx 프로세스가 /var/log/ 이외 경로에 파일 쓰기
  condition: >
    container
    and proc.name = nginx
    and write_file
    and not fd.directory startswith /var/log/nginx
    and not fd.directory startswith /tmp
  output: >
    Nginx wrote to unexpected path
    (pod=%k8s.pod.name path=%fd.name
     cmdline=%proc.cmdline image=%container.image.repository:%container.image.tag)
  priority: WARNING
  tags: [custom, nginx, file_write]
```

---

## 5. Falcosidekick — 알림 라우팅

```
Falco 이벤트
    │
    ▼
Falcosidekick (이벤트 팬아웃)
    ├── Slack          → 채널별 우선순위 필터링
    ├── PagerDuty      → CRITICAL만 온콜 호출
    ├── Elasticsearch  → 이벤트 저장 및 검색
    ├── Prometheus     → 이벤트 카운터 메트릭
    ├── AWS SNS/SQS    → 자동화 대응 파이프라인
    └── Webhook        → 커스텀 대응 시스템
```

```yaml
# Falcosidekick 설정 예시
slack:
  webhookurl: https://hooks.slack.com/services/T.../B.../...
  minimumpriority: warning     # WARNING 이상만 Slack 전송
  channel: "#security-alerts"

pagerduty:
  routingkey: <PagerDuty Routing Key>
  minimumpriority: critical    # CRITICAL만 PagerDuty 호출

elasticsearch:
  hostport: http://elasticsearch:9200
  index: falco-events
  minimumpriority: debug       # 모든 이벤트 저장

prometheus:
  # /metrics 엔드포인트 자동 노출
  # falco_events_total{priority="WARNING", rule="..."} 카운터
```

---

## 6. 자동 대응 (Response Engine)

Falco 이벤트 발생 시 자동으로 조치합니다.

```yaml
# Falcosidekick + Kubeless/Tekton으로 자동 대응
# 이벤트 발생 → Webhook → 람다/함수 실행 → 파드 격리

# 예: 의심스러운 파드 자동 격리
# Falco CRITICAL 이벤트 → Webhook → 다음 스크립트 실행
```

```python
# response-handler.py (Webhook 수신)
import json
from kubernetes import client, config

def handle_falco_event(event):
    if event['priority'] == 'CRITICAL':
        pod_name = event['output_fields']['k8s.pod.name']
        namespace = event['output_fields']['k8s.ns.name']

        # 파드에 격리 레이블 추가 (NetworkPolicy로 트래픽 차단)
        config.load_incluster_config()
        v1 = client.CoreV1Api()
        v1.patch_namespaced_pod(
            name=pod_name,
            namespace=namespace,
            body={"metadata": {"labels": {"security-quarantine": "true"}}}
        )
        print(f"Quarantined pod {pod_name} in {namespace}")
```

```yaml
# 격리된 파드 트래픽 차단 NetworkPolicy
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: quarantine-policy
spec:
  podSelector:
    matchLabels:
      security-quarantine: "true"   # 격리 레이블 달린 파드
  policyTypes:
  - Ingress
  - Egress
  # ingress/egress 없음 → 모든 트래픽 차단
```

---

## 7. 트러블슈팅

* **Falco가 너무 많은 알림 생성 (노이즈):**
  ```bash
  # 어떤 룰이 가장 많이 발생하는지 확인
  kubectl logs -n falco daemonset/falco | \
    jq -r '.rule' | sort | uniq -c | sort -rn | head -20

  # 특정 룰의 허용 목록에 추가
  # rules.yaml에 exception 추가:
  # - rule: Terminal Shell in Container
  #   exceptions:
  #   - name: trusted_shell_containers
  #     fields: [k8s.pod.label.allow-shell]
  #     comps: [=]
  #     values: [["true"]]
  ```

* **eBPF 드라이버 로드 실패:**
  ```bash
  # 커널 버전 확인 (eBPF는 5.8+ 권장)
  uname -r

  # BTF 지원 확인
  ls /sys/kernel/btf/vmlinux

  # 없으면 커널 모듈 드라이버로 전환
  helm upgrade falco falcosecurity/falco \
    --set driver.kind=module
  ```

* **특정 네임스페이스 Falco 감시 제외:**
  ```yaml
  # values.yaml
  falco:
    rules_files:
    - /etc/falco/falco_rules.yaml
    - /etc/falco/custom_rules.yaml

  # custom_rules.yaml에 macro override
  - macro: trusted_namespaces
    condition: >
      k8s.ns.name in (kube-system, monitoring, falco, my-trusted-app)
  ```
