## 1. 개요 및 비유
**Pod Security Standards(PSS)**는 파드의 보안 수준을 세 가지 정책 레벨로 표준화한 쿠버네티스 내장 기능입니다. 네임스페이스 단위로 적용되며, 기존 PodSecurityPolicy(PSP, 1.25에서 제거)를 대체합니다.

💡 **비유하자면 '건물 보안 등급 시스템'과 같습니다.**
일반 사무실(privileged) → 보안 구역(baseline) → 금고실(restricted) 처럼 구역별로 출입 규칙이 다릅니다. 개발 네임스페이스는 규칙을 느슨하게, 프로덕션 네임스페이스는 가장 엄격하게 설정할 수 있습니다.

## 2. 핵심 설명

### 3가지 정책 레벨

| 레벨 | 설명 | 권장 환경 |
|---|---|---|
| **privileged** | 제한 없음. 모든 설정 허용 | 시스템 컴포넌트, kube-system |
| **baseline** | 최소한의 제한. 일반적인 컨테이너 설정 허용, 명백히 위험한 것만 차단 | 개발/스테이징 |
| **restricted** | 강력한 제한. 현재 보안 모범 사례를 모두 적용 | 프로덕션 |

### restricted 레벨에서 강제되는 주요 항목
- `runAsNonRoot: true` 필수
- `allowPrivilegeEscalation: false` 필수
- `capabilities.drop: ["ALL"]` 필수 (add는 `NET_BIND_SERVICE`만 허용)
- `seccompProfile` 설정 필수 (`RuntimeDefault` 또는 `Localhost`)
- `hostNetwork`, `hostPID`, `hostIPC` 사용 불가
- `privileged: true` 사용 불가
- `hostPath` 볼륨 사용 불가

### 3가지 적용 모드

| 모드 | 동작 |
|---|---|
| `enforce` | 정책 위반 파드는 **생성 거부** |
| `audit` | 위반 파드는 **생성 허용**하지만 감사 로그에 기록 |
| `warn` | 위반 파드는 **생성 허용**하지만 사용자에게 **경고 메시지** 표시 |

`audit`/`warn` 모드는 enforce 적용 전 영향 범위를 파악하는 데 사용합니다.

## 3. YAML 적용 예시

### 네임스페이스에 PSS 적용 (라벨 방식)
```bash
# production 네임스페이스에 restricted 정책 enforce
kubectl label namespace production \
  pod-security.kubernetes.io/enforce=restricted \
  pod-security.kubernetes.io/enforce-version=latest

# staging 네임스페이스: baseline enforce + restricted warn/audit
kubectl label namespace staging \
  pod-security.kubernetes.io/enforce=baseline \
  pod-security.kubernetes.io/warn=restricted \
  pod-security.kubernetes.io/audit=restricted
```

### YAML로 네임스페이스 생성과 동시에 적용
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: production
  labels:
    # enforce: 위반 시 거부
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/enforce-version: latest
    # warn: 경고 표시 (더 엄격한 버전으로 예고)
    pod-security.kubernetes.io/warn: restricted
    pod-security.kubernetes.io/warn-version: latest
    # audit: 감사 로그 기록
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/audit-version: latest
```

### restricted 레벨을 통과하는 최소 파드 스펙
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: compliant-pod
  namespace: production
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    seccompProfile:
      type: RuntimeDefault       # restricted에서 필수
  containers:
  - name: app
    image: my-app:1.0
    securityContext:
      allowPrivilegeEscalation: false   # 필수
      readOnlyRootFilesystem: true
      capabilities:
        drop: ["ALL"]                   # 필수
        # add: ["NET_BIND_SERVICE"]     # 필요 시만 추가 (restricted에서 유일하게 허용)
```

### 기존 네임스페이스에 점진적 적용 (마이그레이션 전략)
```bash
# 1단계: 현재 네임스페이스에서 baseline 위반 파드 파악 (enforce 아님)
kubectl label namespace my-app \
  pod-security.kubernetes.io/warn=baseline \
  pod-security.kubernetes.io/audit=baseline

# 2단계: 위반 파드 수정 후 enforce 적용
kubectl label namespace my-app \
  pod-security.kubernetes.io/enforce=baseline

# 3단계: restricted로 단계적 강화
kubectl label namespace my-app \
  pod-security.kubernetes.io/warn=restricted
# 경고 없어지면 enforce=restricted 전환
```

### 현재 위반 현황 빠르게 확인
```bash
# 네임스페이스의 모든 파드를 dry-run으로 검증
kubectl label namespace my-app \
  pod-security.kubernetes.io/enforce=restricted --dry-run=server

# 감사 로그에서 PSS 위반 확인
kubectl get events -n my-app | grep "violates PodSecurity"
```

## 4. 트러블 슈팅

* **restricted 적용 후 기존 파드가 갑자기 안 뜸:**
  * `enforce` 모드는 기존 실행 중인 파드에는 영향 없지만, 재시작/재배포 시 적용됩니다.
  * 먼저 `warn`/`audit` 모드로 변경하고 경고/로그를 분석하여 위반 항목을 하나씩 수정한 후 `enforce`로 전환하세요.

* **kube-system 네임스페이스에 PSS 적용하면 안 됨:**
  * `coredns`, `kube-proxy` 등 시스템 파드는 `hostNetwork` 등 restricted에서 금지된 설정을 사용합니다. 시스템 네임스페이스에는 `privileged` 레벨을 유지하세요.

* **특정 파드만 예외 처리하고 싶을 때:**
  * PSS는 파드 단위 예외를 지원하지 않습니다. 파드 단위 예외가 필요하다면 **Kyverno**나 **OPA Gatekeeper**를 사용하세요.
  * 또는 해당 파드를 별도의 네임스페이스(예: `privileged-workloads`)로 분리하고 다른 정책을 적용하는 방법도 있습니다.

* **`Error: pods "..." is forbidden: violates PodSecurity` 메시지에서 원인 파악:**
  ```bash
  # 메시지에 위반 항목이 명시되어 있음, 예:
  # violates PodSecurity "restricted:latest":
  #   allowPrivilegeEscalation != false (container "app")
  #   unrestricted capabilities (container "app" must set securityContext.capabilities.drop=["ALL"])
  ```
  메시지에서 위반 항목을 확인하고 해당 SecurityContext 설정을 추가하세요.
