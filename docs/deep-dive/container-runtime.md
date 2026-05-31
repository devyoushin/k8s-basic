## 1. 개요 및 비유
**컨테이너 런타임**은 실제로 컨테이너를 실행하고 관리하는 소프트웨어입니다. Kubernetes는 런타임과 직접 대화하지 않고, **CRI(Container Runtime Interface)**라는 표준 인터페이스를 통해 통신합니다. 현재 표준 런타임은 **containerd**입니다.

💡 **비유하자면 '운영체제와 앱 사이의 번역기'와 같습니다.**
Kubelet(현장소장)이 "방 하나 만들어"라고 지시하면, containerd(건설업체)가 설계도(OCI 이미지)를 받아 실제 방(컨테이너)을 짓습니다. 어떤 건설업체를 쓰든 Kubelet은 같은 방식으로 지시할 수 있도록 CRI라는 표준 계약서가 존재합니다.

## 2. 핵심 설명

### 컨테이너 런타임 스택

```
kubectl / API Server
        │
    Kubelet
        │  CRI (gRPC)
    containerd          ← 현재 k8s 표준 (Docker 제거 후)
        │  OCI Runtime Spec
      runc              ← 실제 컨테이너 프로세스 생성 (namespace, cgroup 설정)
        │
    컨테이너 프로세스
```

### 표준 스펙 정리

| 표준 | 설명 |
|---|---|
| **CRI** (Container Runtime Interface) | Kubelet ↔ 런타임 간 통신 규약 (gRPC). containerd, CRI-O가 구현체 |
| **OCI** (Open Container Initiative) | 이미지 포맷 + 런타임 스펙. Docker와 containerd가 모두 호환 |
| **runc** | OCI Runtime Spec의 참조 구현체. 실제 namespace/cgroup 설정 수행 |

### Docker vs containerd
Kubernetes 1.24부터 Docker가 제거됐습니다. 하지만 **Docker로 빌드한 이미지는 그대로 사용 가능**합니다. 이미지 포맷(OCI)이 동일하기 때문입니다.

| 항목 | Docker (구) | containerd (현) |
|---|---|---|
| Kubelet 통신 | dockershim (별도 레이어) | CRI 직접 통신 |
| 오버헤드 | 높음 (레이어 추가) | 낮음 |
| 이미지 호환성 | OCI 표준 | OCI 표준 (동일) |
| CLI 도구 | `docker` | `ctr`, `crictl`, `nerdctl` |

### containerd 아키텍처

```
containerd 데몬
├── snapshotter    : 이미지 레이어 관리 (overlay filesystem)
├── content store  : 이미지 blob 저장
├── runtime        : runc 실행 관리
└── shim           : 각 컨테이너의 생명주기 관리 (containerd-shim-runc-v2)
```

**containerd shim의 역할:** containerd가 재시작되어도 이미 실행 중인 컨테이너가 죽지 않도록 컨테이너와 런타임 사이의 중간 프로세스로 남아있습니다.

### 보안 강화 런타임 (gVisor, Kata Containers)

일반 `runc`는 호스트 커널을 공유하므로 컨테이너 탈출(escape) 취약점이 발생할 수 있습니다. 더 강한 격리가 필요한 경우 아래를 사용합니다.

| 런타임 | 격리 방식 | 트레이드오프 |
|---|---|---|
| `runc` (기본) | 리눅스 namespace/cgroup | 빠름, 호스트 커널 공유 |
| `gVisor (runsc)` | 사용자 공간 커널 (ptrace/KVM) | 느림, 강한 격리 |
| `Kata Containers` | 경량 VM (QEMU/Firecracker) | 가장 강한 격리, 가장 느림 |

## 3. 실습 예시

### RuntimeClass로 보안 런타임 선택
```yaml
# gVisor RuntimeClass 등록
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: gvisor
handler: runsc   # containerd에 등록된 런타임 핸들러 이름

---
# 파드에서 RuntimeClass 지정
apiVersion: v1
kind: Pod
metadata:
  name: secure-pod
spec:
  runtimeClassName: gvisor  # 이 파드만 gVisor로 실행
  containers:
  - name: app
    image: my-app:1.0
```

### crictl — 노드에서 컨테이너 직접 디버깅
```bash
# containerd가 관리 중인 모든 컨테이너 조회 (docker ps 대체)
crictl ps

# 파드 목록 조회
crictl pods

# 컨테이너 로그 확인
crictl logs <컨테이너ID>

# 이미지 목록 확인
crictl images

# 컨테이너 내부 진입
crictl exec -it <컨테이너ID> /bin/sh

# containerd 소켓 경로 확인
ls -la /run/containerd/containerd.sock
```

### nerdctl — containerd용 Docker 호환 CLI
```bash
# Docker와 동일한 인터페이스로 containerd 직접 조작
nerdctl run -it --rm alpine sh
nerdctl build -t my-app:1.0 .
nerdctl push my-app:1.0
```

### 컨테이너 런타임 확인
```bash
# 노드의 컨테이너 런타임 확인
kubectl get nodes -o wide
# CONTAINER-RUNTIME 컬럼에서 containerd://1.7.x 확인

# 특정 노드 상세 정보
kubectl describe node <노드명> | grep "Container Runtime"
```

## 4. 트러블 슈팅

* **`docker` 명령어가 노드에서 안 됨 (k8s 1.24+):**
  * 의도된 변경입니다. 노드에서 컨테이너를 직접 조작하려면 `crictl`을 사용하세요.
  * `alias docker=nerdctl` 처럼 nerdctl을 설치하면 기존 습관대로 사용할 수 있습니다.

* **파드가 뜨는데 `ImagePullBackOff` 발생:**
  ```bash
  # containerd가 이미지를 직접 pull할 수 있는지 확인
  crictl pull <이미지명>
  # 인증이 필요하면
  crictl pull --creds user:password <이미지명>
  ```

* **containerd 데몬이 응답 없음:**
  ```bash
  systemctl status containerd
  journalctl -u containerd -n 100 --no-pager
  # 재시작 시 이미 실행 중인 컨테이너는 shim이 유지하므로 영향 없음
  systemctl restart containerd
  ```

* **노드의 디스크가 이미지로 꽉 참:**
  ```bash
  # 사용하지 않는 이미지 정리
  crictl rmi --prune

  # containerd 이미지 전체 목록과 크기 확인
  ctr -n k8s.io images ls
  ```
