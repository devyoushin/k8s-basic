## 1. 개요 및 비유

컨테이너는 VM과 달리 **호스트 OS 커널을 직접 공유**합니다. containerd와 runc는 리눅스 커널의 격리 기능들을 조합하여 프로세스를 "컨테이너처럼" 보이게 만듭니다.

💡 **비유하자면 오피스 건물의 파티션(칸막이)과 같습니다.**
각 입주사(컨테이너)는 독립된 공간처럼 보이지만, 전기(CPU), 배관(메모리), 건물 관리자(커널)는 공유합니다. VM은 아예 다른 건물을 짓는 것입니다.

---

## 2. 커널 격리 기술 심층 분석

### 2.1 전체 스택 — 커널까지의 흐름

```
kubelet
  │  CRI gRPC
  ▼
containerd 데몬
  │  OCI Runtime Spec (config.json 생성)
  ▼
containerd-shim-runc-v2   ← 컨테이너 수명 관리 프로세스
  │
  ▼
runc (컨테이너 생성기)
  │  clone() 시스템 콜
  ▼
Linux Kernel
  ├── Namespaces   (격리: 무엇이 보이는가)
  ├── Cgroups      (제한: 얼마나 쓸 수 있는가)
  ├── Seccomp      (필터: 어떤 시스템 콜을 허용하는가)
  ├── Capabilities (권한: 어떤 특권이 있는가)
  └── OverlayFS    (파일시스템: 이미지 레이어 합성)
```

### 2.2 Linux Namespaces — 격리의 핵심

runc는 컨테이너 시작 시 `clone()` 시스템 콜에 플래그를 조합하여 새 네임스페이스를 생성합니다.

| 네임스페이스 | 플래그 | 격리 대상 | 컨테이너에서 보이는 것 |
|---|---|---|---|
| **PID** | `CLONE_NEWPID` | 프로세스 ID | 컨테이너 내 첫 프로세스가 PID 1 |
| **NET** | `CLONE_NEWNET` | 네트워크 인터페이스, IP | 자체 eth0, lo 인터페이스 |
| **MNT** | `CLONE_NEWNS` | 마운트 포인트 | 독립된 파일시스템 뷰 |
| **UTS** | `CLONE_NEWUTS` | hostname, domainname | 독립된 hostname 설정 가능 |
| **IPC** | `CLONE_NEWIPC` | System V IPC, 메시지 큐 | 격리된 IPC 자원 |
| **USER** | `CLONE_NEWUSER` | UID/GID 매핑 | 컨테이너 내 root → 호스트의 일반 유저 |
| **CGROUP** | `CLONE_NEWCGROUP` | cgroup 루트 | 자신의 cgroup 계층만 보임 |

```bash
# 컨테이너 프로세스의 네임스페이스 확인
PID=$(docker inspect --format '{{.State.Pid}}' <컨테이너ID>)
ls -la /proc/$PID/ns/
# 출력 예:
# lrwxrwxrwx 1 root root 0 ... ipc -> ipc:[4026532345]
# lrwxrwxrwx 1 root root 0 ... net -> net:[4026532347]
# lrwxrwxrwx 1 root root 0 ... pid -> pid:[4026532346]

# 호스트와 비교 (다른 inode 번호 = 다른 네임스페이스)
ls -la /proc/1/ns/

# 특정 네임스페이스에 진입 (nsenter)
nsenter --target $PID --pid --mount --net -- /bin/bash
# → 컨테이너 내부와 동일한 뷰를 호스트에서 확인 가능
```

#### PID Namespace 상세

```
호스트 PID 공간           컨테이너 PID 공간
─────────────────         ─────────────────
PID 1: systemd            PID 1: nginx (컨테이너 init)
PID 234: containerd       PID 2: nginx worker
PID 891: shim
PID 892: nginx  ←─────── 동일한 커널 프로세스이지만
PID 893: nginx worker     다른 PID 번호로 보임

# 호스트에서 컨테이너 프로세스 확인
ps aux | grep nginx
# PID 892, 893으로 표시

# 컨테이너 내부에서 확인
kubectl exec my-pod -- ps aux
# PID 1, 2로 표시
```

#### Network Namespace 상세

```
컨테이너 NET 네임스페이스 생성 흐름 (파드 생성 시):
┌────────────────────────────────────────────────────────┐
│ 1. kubelet이 pause 컨테이너(infra 컨테이너) 먼저 생성  │
│    → 이 pause 컨테이너가 NET 네임스페이스 소유          │
│                                                        │
│ 2. CNI 플러그인 호출 (예: Calico, Flannel)             │
│    → pause 컨테이너의 NET 네임스페이스에               │
│       가상 이더넷 쌍(veth pair) 생성                  │
│       veth0 ─────────── veth1                         │
│       (컨테이너 eth0)    (호스트 브릿지에 연결)         │
│                                                        │
│ 3. 같은 파드의 모든 컨테이너가 pause의 NET 네임스페이스 │
│    공유 → localhost로 서로 통신 가능                   │
└────────────────────────────────────────────────────────┘
```

```bash
# 컨테이너의 veth 쌍 확인
# 컨테이너 내부에서:
kubectl exec my-pod -- ip addr show eth0
# 출력: ... link/ether <mac>

# 호스트에서 대응하는 veth 인터페이스 찾기
ip link | grep -A1 "veth"

# 컨테이너의 네트워크 네임스페이스 직접 진입
nsenter --target $PID --net -- ip route
```

### 2.3 Cgroups (Control Groups) — 자원 제한

Cgroups는 프로세스 그룹의 **CPU, 메모리, 디스크 I/O, 네트워크** 사용량을 제한하고 측정합니다.

```
cgroup 계층 구조 (v2 기준, /sys/fs/cgroup):
/sys/fs/cgroup/
└── kubepods/
    ├── burstable/
    │   └── pod<UID>/
    │       ├── cpu.max          ← CPU 쿼터 설정
    │       ├── memory.max       ← 메모리 한도
    │       └── <컨테이너ID>/
    │           ├── cpu.max
    │           └── memory.current   ← 현재 사용량
    └── guaranteed/
        └── pod<UID>/...
```

```bash
# 특정 파드의 cgroup 경로 확인
POD_UID=$(kubectl get pod my-pod -o jsonpath='{.metadata.uid}')
ls /sys/fs/cgroup/kubepods/burstable/pod${POD_UID}/

# CPU 제한 확인 (cgroup v2)
cat /sys/fs/cgroup/kubepods/burstable/pod${POD_UID}/*/cpu.max
# 출력: 100000 100000
# 형식: <quota_us> <period_us>
# 100000/100000 = 1 CPU 코어 (100%)

# 메모리 제한 확인
cat /sys/fs/cgroup/kubepods/burstable/pod${POD_UID}/*/memory.max
# 출력: 536870912  (512Mi in bytes)

# 실시간 메모리 사용량
cat /sys/fs/cgroup/kubepods/burstable/pod${POD_UID}/*/memory.current
```

#### CPU 제한의 실제 동작

```yaml
# YAML에서 선언하는 값
resources:
  requests:
    cpu: "500m"      # 0.5 CPU 코어 예약
    memory: "256Mi"
  limits:
    cpu: "1000m"     # 1 CPU 코어 최대
    memory: "512Mi"

# → 커널 cgroup 설정으로 변환:
# cpu.shares = 512       (requests 기반 상대적 가중치, 1 CPU = 1024)
# cpu.max = 100000 100000 (limits 기반: 100ms 마다 100ms quota = 1 CPU)
# memory.max = 536870912  (512 * 1024 * 1024 bytes)
```

```bash
# CPU Throttling 확인 (컨테이너가 CPU 제한에 걸리는지)
cat /sys/fs/cgroup/kubepods/.../cpu.stat
# 출력:
# usage_usec 12345678
# user_usec 9876543
# system_usec 2469135
# nr_periods 1000       ← 총 스케줄링 주기 횟수
# nr_throttled 150      ← 스로틀된 횟수
# throttled_usec 300000 ← 스로틀된 총 시간(마이크로초)

# Prometheus로 확인
# container_cpu_cfs_throttled_periods_total
# container_cpu_cfs_periods_total
```

### 2.4 OverlayFS — 컨테이너 파일시스템

컨테이너 이미지는 **읽기 전용 레이어들의 합성**입니다. containerd는 OverlayFS를 사용합니다.

```
OverlayFS 레이어 구조:
┌────────────────────────────────────────┐
│  Upper Layer (읽기/쓰기 — 컨테이너 전용) │  ← 파일 변경 시 여기에 기록 (CoW)
├────────────────────────────────────────┤
│  Lower Layer 3: 앱 레이어 (읽기 전용)   │  ← Dockerfile COPY ./app /app
├────────────────────────────────────────┤
│  Lower Layer 2: 의존성 레이어 (읽기 전용)│  ← RUN pip install -r requirements.txt
├────────────────────────────────────────┤
│  Lower Layer 1: 베이스 이미지 (읽기 전용)│  ← FROM ubuntu:22.04
└────────────────────────────────────────┘
         ↕ 커널 OverlayFS가 통합된 단일 뷰 제공
    마운트 포인트 /merged (컨테이너에서 / 로 보임)
```

```bash
# containerd가 사용하는 snapshot 위치
ls /var/lib/containerd/io.containerd.snapshotter.v1.overlayfs/snapshots/

# 실행 중인 컨테이너의 OverlayFS 마운트 확인
mount | grep overlay
# 출력 예:
# overlay on /run/containerd/io.containerd.runtime.v2.task/.../rootfs type overlay
# (rw,lowerdir=/var/lib/.../layer3:/var/lib/.../layer2:/var/lib/.../layer1,
#  upperdir=/var/lib/.../upper,
#  workdir=/var/lib/.../work)

# Copy-on-Write 확인: 컨테이너 내부에서 파일 수정 후
kubectl exec my-pod -- touch /etc/test-file
# upper 디렉토리에 변경사항이 기록됨
ls /var/lib/containerd/.../upper/etc/
```

### 2.5 Seccomp — 시스템 콜 필터링

컨테이너는 호스트 커널을 공유하므로, 위험한 시스템 콜을 BPF 필터로 차단합니다.

```
시스템 콜 흐름:
컨테이너 프로세스
   │ int 0x80 / syscall 인스트럭션
   ▼
커널 진입점
   │
   ├─[Seccomp BPF 필터]──────────────────────────────────┐
   │  허용된 syscall?                                      │
   │  YES → 계속 진행                                     │
   │  NO  → EPERM 반환 또는 프로세스 강제 종료 (SIGSYS)   │
   └──────────────────────────────────────────────────────┘
   │
   ▼
시스템 콜 핸들러 실행
```

```yaml
# Kubernetes 기본 Seccomp 프로파일 적용
apiVersion: v1
kind: Pod
spec:
  securityContext:
    seccompProfile:
      type: RuntimeDefault   # containerd의 기본 프로파일 사용
      # 또는 Localhost로 커스텀 프로파일 지정 가능

# 커스텀 프로파일 (특정 syscall만 허용)
# type: Localhost
# localhostProfile: profiles/my-app.json
```

```bash
# 기본 seccomp 프로파일 위치 (containerd)
cat /etc/containerd/config.toml | grep seccomp

# 차단된 syscall 확인 (audit 모드로 먼저 테스트)
# strace로 컨테이너 프로세스의 syscall 확인
strace -p $PID -e trace=all 2>&1 | head -50
```

### 2.6 Linux Capabilities — root 권한 세분화

기존의 root(전능) vs 일반유저 이분법 대신, 권한을 31개 카테고리로 분리합니다.

```
일반 컨테이너의 기본 Capabilities (제한적 집합):
┌─────────────────────────────────────┐
│ CAP_NET_BIND_SERVICE  ← 80번 포트   │
│ CAP_CHOWN             ← chown 허용  │
│ CAP_SETUID/SETGID     ← UID 변경   │
│ CAP_KILL              ← 시그널 발송 │
│ ...                                 │
└─────────────────────────────────────┘

제거된 위험 Capabilities:
✗ CAP_SYS_ADMIN      ← mount, 여러 관리 작업
✗ CAP_NET_ADMIN      ← 네트워크 인터페이스 설정
✗ CAP_SYS_PTRACE     ← 다른 프로세스 디버깅
✗ CAP_DAC_OVERRIDE   ← 파일 권한 우회
```

```yaml
# Capabilities 세밀 제어
spec:
  containers:
  - name: app
    securityContext:
      capabilities:
        drop:
          - ALL              # 모든 권한 제거
        add:
          - NET_BIND_SERVICE # 필요한 것만 추가
      runAsNonRoot: true
      runAsUser: 1000
      allowPrivilegeEscalation: false
```

```bash
# 컨테이너 프로세스의 현재 Capabilities 확인
kubectl exec my-pod -- cat /proc/1/status | grep Cap
# 출력:
# CapInh: 0000000000000000  (상속 가능)
# CapPrm: 00000000a80425fb  (허용됨)
# CapEff: 00000000a80425fb  (실제 활성)
# CapBnd: 00000000a80425fb  (바운딩 셋)

# 16진수를 사람이 읽을 수 있는 형태로 변환
capsh --decode=00000000a80425fb
```

---

## 3. YAML 적용 예시

### 최소 권한 컨테이너 (Best Practice)

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: hardened-pod
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    runAsGroup: 3000
    fsGroup: 2000
    seccompProfile:
      type: RuntimeDefault   # 기본 seccomp 프로파일
  containers:
  - name: app
    image: my-app:1.0
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        cpu: "500m"
        memory: "256Mi"
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true   # OverlayFS upper 레이어도 읽기 전용
      capabilities:
        drop:
          - ALL
    volumeMounts:
    - name: tmp
      mountPath: /tmp              # 쓰기 필요한 경로만 별도 볼륨
  volumes:
  - name: tmp
    emptyDir: {}
```

---

## 4. 트러블슈팅

* **컨테이너가 `Operation not permitted` 오류:**
  ```bash
  # 어떤 capability가 필요한지 확인
  kubectl exec my-pod -- strace <명령어> 2>&1 | grep EPERM
  # 또는 audit 로그에서 seccomp 차단 확인
  dmesg | grep "audit: type=1326"  # SECCOMP 차단 로그
  ```

* **메모리 OOM Killed (컨테이너 재시작 반복):**
  ```bash
  # OOM 이벤트 확인
  kubectl describe pod my-pod | grep -A5 "OOMKilled"

  # 호스트 커널 OOM 로그 확인
  dmesg | grep -i "oom\|killed process"

  # cgroup 메모리 이벤트 확인
  cat /sys/fs/cgroup/.../memory.events
  # oom_kill 카운터가 증가했는지 확인
  ```

* **CPU Throttling으로 응답 느림:**
  ```bash
  # cgroup v2에서 스로틀링 비율 계산
  cat /sys/fs/cgroup/.../cpu.stat
  # nr_throttled / nr_periods * 100 = 스로틀링 비율(%)
  # 25% 이상이면 CPU limits 상향 고려

  # limits 없이 requests만 설정하면 스로틀링 없음
  # (노드 여유 자원 범위 내에서 버스트 가능)
  ```

* **OverlayFS 마운트 실패 (`failed to mount overlay`):**
  ```bash
  # 커널 OverlayFS 지원 확인
  grep overlay /proc/filesystems
  # 없으면: modprobe overlay

  # d_type 지원 확인 (XFS에서 필요)
  xfs_info /var/lib/containerd | grep ftype
  # ftype=1 이어야 함
  ```

* **`/proc` 또는 `/sys` 접근 필요한 컨테이너 (모니터링 에이전트 등):**
  ```yaml
  # 호스트 PID 네임스페이스 공유 (DaemonSet에서 주로 사용)
  spec:
    hostPID: true
    hostNetwork: true
    volumes:
    - name: proc
      hostPath:
        path: /proc
    - name: sys
      hostPath:
        path: /sys
  ```
