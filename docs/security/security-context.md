## 1. 개요 및 비유
**SecurityContext(보안 컨텍스트)**는 파드 또는 컨테이너가 실행될 때의 보안 설정을 정의합니다. 어떤 유저로 실행할지, 루트 권한을 막을지, Linux 커널 기능(Capabilities)을 제어할지 등을 설정합니다.

💡 **비유하자면 '직원의 사원증 등급과 출입 구역 제한'과 같습니다.**
기본적으로 컨테이너는 root로 실행되는데, 이는 사원증 없이 모든 문이 열린 것과 같습니다. SecurityContext는 "이 직원은 일반 직원 권한(UID 1000)으로만 들어와야 하고, 금고실(특권 모드)은 절대 못 들어간다"는 규칙을 강제합니다.

## 2. 핵심 설명

### Pod 레벨 vs Container 레벨
- **`spec.securityContext`**: 파드 내 모든 컨테이너에 적용 (fsGroup, runAsUser 등)
- **`spec.containers[].securityContext`**: 특정 컨테이너에만 적용, Pod 레벨 설정을 덮어씁니다

### 핵심 설정 항목

| 설정 | 설명 | 권장값 |
|---|---|---|
| `runAsNonRoot` | root(UID 0) 실행 차단 | `true` |
| `runAsUser` | 실행할 UID 지정 | `1000` 이상 |
| `runAsGroup` | 실행할 GID 지정 | `1000` 이상 |
| `readOnlyRootFilesystem` | 루트 파일시스템 읽기 전용 | `true` |
| `allowPrivilegeEscalation` | 자식 프로세스의 권한 상승 차단 | `false` |
| `privileged` | 호스트와 동일한 권한 부여 | `false` (절대 `true` 금지) |
| `capabilities` | Linux 커널 기능 추가/제거 | `drop: ["ALL"]` 후 필요한 것만 add |
| `seccompProfile` | 허용할 시스템 콜 필터링 | `RuntimeDefault` 또는 커스텀 프로파일 |
| `fsGroup` | 마운트된 볼륨의 그룹 소유권 | 앱 GID |

### Linux Capabilities 주요 목록
컨테이너는 기본적으로 일부 Capabilities를 갖고 시작합니다. `drop: ["ALL"]` 후 필요한 것만 명시하는 것이 원칙입니다.

| Capability | 하는 일 | 필요한 경우 |
|---|---|---|
| `NET_BIND_SERVICE` | 1024 미만 포트 바인딩 허용 | 80/443 포트 직접 사용 시 |
| `NET_ADMIN` | 네트워크 설정 변경 | CNI 플러그인, 네트워크 툴 |
| `SYS_PTRACE` | 다른 프로세스 추적 | 디버깅 도구 |
| `CHOWN` | 파일 소유권 변경 | 일부 초기화 스크립트 |
| `DAC_OVERRIDE` | 파일 권한 무시 | (피해야 함) |

## 3. YAML 적용 예시

### 권장 보안 설정 (일반 웹 서버)
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: secure-web
spec:
  template:
    spec:
      # Pod 레벨: 모든 컨테이너에 적용
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000           # 볼륨 마운트 시 그룹 소유권 설정
        seccompProfile:
          type: RuntimeDefault  # 컨테이너 런타임 기본 seccomp 프로파일 사용

      containers:
      - name: web
        image: my-web:1.0
        # Container 레벨: 이 컨테이너에만 적용
        securityContext:
          allowPrivilegeEscalation: false   # sudo, setuid 바이너리 실행 차단
          readOnlyRootFilesystem: true       # 루트 파일시스템 쓰기 차단
          capabilities:
            drop: ["ALL"]                    # 모든 커널 기능 제거
            add: ["NET_BIND_SERVICE"]        # 80포트 사용 위해 이것만 추가
        ports:
        - containerPort: 80
        volumeMounts:
        - name: tmp             # readOnlyRootFilesystem이면 /tmp도 마운트 필요
          mountPath: /tmp
        - name: cache
          mountPath: /var/cache/nginx

      volumes:
      - name: tmp
        emptyDir: {}
      - name: cache
        emptyDir: {}
```

### 커스텀 Seccomp 프로파일
```yaml
# 특정 시스템 콜만 허용하는 커스텀 프로파일
# 노드의 /var/lib/kubelet/seccomp/profiles/custom.json 에 저장

securityContext:
  seccompProfile:
    type: Localhost
    localhostProfile: profiles/custom.json
```

```json
// /var/lib/kubelet/seccomp/profiles/custom.json
{
  "defaultAction": "SCMP_ACT_ERRNO",
  "architectures": ["SCMP_ARCH_X86_64"],
  "syscalls": [
    {
      "names": ["read", "write", "open", "close", "stat", "fstat",
                "mmap", "mprotect", "munmap", "brk", "rt_sigaction",
                "rt_sigprocmask", "ioctl", "access", "pipe", "select",
                "sched_yield", "mremap", "msync", "mincore", "madvise",
                "dup", "dup2", "nanosleep", "alarm", "getpid", "socket",
                "connect", "accept", "sendto", "recvfrom", "sendmsg",
                "recvmsg", "bind", "listen", "getsockname", "getpeername",
                "clone", "fork", "execve", "exit", "wait4", "kill",
                "uname", "fcntl", "getcwd", "chdir", "exit_group",
                "set_tid_address", "futex", "set_robust_list"],
      "action": "SCMP_ACT_ALLOW"
    }
  ]
}
```

### AppArmor 프로파일 적용 (노드에 프로파일 로드 필요)
```yaml
metadata:
  annotations:
    container.apparmor.security.beta.kubernetes.io/web: localhost/k8s-deny-write
spec:
  containers:
  - name: web
    image: nginx:1.25
```

## 4. 트러블 슈팅

* **`container has runAsNonRoot and image will run as root` 에러:**
  * 컨테이너 이미지의 기본 USER가 root입니다. `runAsUser: 1000` 을 명시하거나 Dockerfile에서 `USER 1000`을 추가하세요.

* **`readOnlyRootFilesystem: true` 설정 후 앱이 죽음:**
  * 앱이 루트 파일시스템에 직접 쓰는 경로가 있는 것입니다. (`/tmp`, `/var/run`, `/var/log` 등)
  * 해당 경로를 `emptyDir` 볼륨으로 마운트하여 쓰기를 허용하세요.
  ```bash
  # 어떤 경로에 쓰는지 확인
  kubectl exec <파드명> -- strace -e trace=openat <앱 실행 명령>
  ```

* **`capability` 관련 Operation not permitted 에러:**
  * `drop: ["ALL"]` 이후 앱이 필요로 하는 capability가 제거된 것입니다.
  * `strace`나 에러 메시지로 어떤 시스템 콜이 차단됐는지 확인하고 필요한 capability만 `add`에 추가하세요.

* **`allowPrivilegeEscalation: false` 설정 후 특정 바이너리가 동작 안 함:**
  * `setuid` 비트가 설정된 바이너리(`ping`, `sudo` 등)는 권한 상승이 필요합니다. 해당 바이너리를 제거하거나 `NET_RAW` capability 추가를 검토하세요.
