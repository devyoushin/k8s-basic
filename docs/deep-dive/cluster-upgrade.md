## 1. 개요 및 비유

클러스터 업그레이드는 Kubernetes에서 가장 긴장되는 운영 작업입니다. 잘못된 순서나 준비 부족은 서비스 중단으로 이어질 수 있습니다.

💡 **비유하자면 고속도로를 달리는 버스의 타이어 교체와 같습니다.**
버스를 완전히 세우지 않고(서비스 중단 없이) 타이어를 한 바퀴씩 교체합니다. 컨트롤 플레인 → 노드 순서가 반드시 지켜져야 합니다.

---

## 2. 업그레이드 원칙과 제약

### 2.1 버전 스큐(Skew) 정책

```
Kubernetes 버전 스큐 정책 (반드시 준수):

kube-apiserver: 항상 가장 최신 버전이어야 함
  ↓ 최대 1 마이너 버전 낮을 수 있음
kube-controller-manager, kube-scheduler
  ↓ 최대 1 마이너 버전 낮을 수 있음
kubelet
  ↓ 최대 2 마이너 버전 낮을 수 있음
kubectl

예: API Server 1.28이면
  - controller-manager: 1.27~1.28 가능
  - kubelet: 1.26~1.28 가능

→ 한 번에 1 마이너 버전씩만 업그레이드 권장
   (1.26 → 1.27 → 1.28, 1.26 → 1.28 바로 불가)
```

### 2.2 업그레이드 순서

```
필수 순서:
1. 컨트롤 플레인 노드 (master) 업그레이드
   - API Server 먼저, 나머지 컴포넌트 따라옴
2. 워커 노드 업그레이드
   - 하나씩 drain → 업그레이드 → uncordon

절대 금지:
- 워커 노드를 컨트롤 플레인보다 먼저 업그레이드
- 한 번에 2 마이너 버전 건너뜀
- HA 클러스터에서 모든 컨트롤 플레인 동시 업그레이드
```

---

## 3. kubeadm 업그레이드 전체 절차

### 3.1 사전 준비

```bash
# 1. 현재 버전 확인
kubectl version
kubeadm version
kubelet --version

# 2. 업그레이드 가능 버전 확인
apt-get update
apt-cache madison kubeadm
# 또는
yum list --showduplicates kubeadm

# 3. etcd 백업 (필수!)
ETCDCTL_API=3 etcdctl snapshot save /backup/pre-upgrade-etcd.db \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key

# 4. 업그레이드 계획 확인 (실제 변경 없음)
kubeadm upgrade plan
# 출력: 업그레이드 가능 버전, 변경될 이미지, 주의 사항 표시
```

### 3.2 컨트롤 플레인 업그레이드

```bash
# ── 컨트롤 플레인 노드에서 실행 ──

# 1. kubeadm 업그레이드
apt-mark unhold kubeadm
apt-get update && apt-get install -y kubeadm=1.28.0-00
apt-mark hold kubeadm

# 2. 업그레이드 적용
kubeadm upgrade apply v1.28.0
# → API Server, Controller Manager, Scheduler, etcd 파드 순차 재시작
# → 각 컴포넌트가 새 이미지로 교체됨 (static pod 업데이트)
# → 약 2~5분 소요

# HA 클러스터의 두 번째, 세 번째 컨트롤 플레인:
kubeadm upgrade node   # apply 대신 node 사용

# 3. kubelet & kubectl 업그레이드
apt-mark unhold kubelet kubectl
apt-get install -y kubelet=1.28.0-00 kubectl=1.28.0-00
apt-mark hold kubelet kubectl

# 4. kubelet 재시작
systemctl daemon-reload
systemctl restart kubelet

# 5. 컨트롤 플레인 정상 확인
kubectl get nodes
kubectl get pods -n kube-system
```

### 3.3 워커 노드 업그레이드 (하나씩 반복)

```bash
# ── 컨트롤 플레인에서 실행 ──

# 1. 노드 드레인 (파드 축출 + 새 파드 배치 차단)
kubectl drain worker-1 \
  --ignore-daemonsets \     # DaemonSet 파드는 무시 (삭제 안 함)
  --delete-emptydir-data \  # emptyDir 볼륨 데이터 삭제 허용
  --force                   # 컨트롤러 없는 파드도 강제 축출
# → 파드들이 다른 노드로 이동됨

# 드레인 완료 확인
kubectl get node worker-1   # STATUS: Ready,SchedulingDisabled

# ── worker-1 노드에서 실행 ──

# 2. kubeadm 업그레이드
apt-mark unhold kubeadm
apt-get install -y kubeadm=1.28.0-00
apt-mark hold kubeadm

kubeadm upgrade node        # 워커 노드는 이 명령 사용

# 3. kubelet 업그레이드
apt-mark unhold kubelet kubectl
apt-get install -y kubelet=1.28.0-00 kubectl=1.28.0-00
apt-mark hold kubelet kubectl
systemctl daemon-reload && systemctl restart kubelet

# ── 컨트롤 플레인에서 실행 ──

# 4. 노드 스케줄링 재개
kubectl uncordon worker-1

# 5. 노드 Ready 확인 후 다음 노드로
kubectl get nodes
# 모든 노드 버전 확인
kubectl get nodes -o custom-columns=NAME:.metadata.name,VERSION:.status.nodeInfo.kubeletVersion
```

---

## 4. 노드 교체 방식 업그레이드 (클라우드 환경)

kubeadm 업그레이드 대신 **새 버전 노드를 추가하고 기존 노드를 삭제**하는 방식입니다.

```
Blue-Green 노드 교체 흐름:
┌──────────────────────────────────────────────────────┐
│ 1. 새 버전(v1.28) AMI/이미지로 새 노드 그룹 생성     │
│    (새 노드들이 클러스터에 자동 조인)                 │
│                                                      │
│ 2. 기존 노드(v1.27)에 Taint 추가                     │
│    kubectl taint node worker-old lifecycle=draining:NoSchedule
│                                                      │
│ 3. 기존 노드 순차 drain                              │
│    → 파드들이 새 노드(v1.28)로 자동 이동            │
│                                                      │
│ 4. 기존 노드 삭제                                    │
│    kubectl delete node worker-old                   │
│    (클라우드 콘솔에서 EC2/VM 인스턴스 종료)          │
└──────────────────────────────────────────────────────┘

장점: 롤백이 쉬움 (기존 노드 그룹 유지하면 됨)
단점: 업그레이드 동안 노드 비용 2배
```

---

## 5. 업그레이드 시 주의 사항

### 5.1 PodDisruptionBudget 충돌

```bash
# drain 중 PDB 위반으로 중단되는 경우
kubectl drain worker-1 --ignore-daemonsets
# 에러: Cannot evict pod as it would violate the pod's disruption budget.

# 방법 1: 다른 노드의 파드가 Ready 될 때까지 대기
# (자동 재시도됨, 최대 --timeout 시간)

# 방법 2: 임시로 PDB minAvailable 낮추기 (위험)
kubectl patch pdb my-pdb -p '{"spec":{"minAvailable":0}}'
# drain 후 원복 필수
kubectl patch pdb my-pdb -p '{"spec":{"minAvailable":1}}'
```

### 5.2 API 버전 제거 대응

```bash
# 업그레이드 전 deprecated/removed API 사용 여부 확인
kubectl api-resources --verbs=list --namespaced -o name | \
  xargs -I {} kubectl get {} --all-namespaces 2>&1 | grep "deprecated"

# pluto 도구로 deprecated API 스캔
pluto detect-files -d ./manifests/
pluto detect-helm --helm-version 3

# 발견된 경우: 매니페스트를 새 API 버전으로 변환
kubectl convert -f old-manifest.yaml --output-version apps/v1
```

---

## 6. 트러블슈팅

* **업그레이드 후 API Server가 시작 안 됨:**
  ```bash
  # static pod 로그 확인
  crictl logs $(crictl ps -a | grep kube-apiserver | awk '{print $1}')

  # manifest 문제인 경우
  cat /etc/kubernetes/manifests/kube-apiserver.yaml

  # etcd 연결 문제인 경우
  ETCDCTL_API=3 etcdctl endpoint health \
    --cacert=/etc/kubernetes/pki/etcd/ca.crt \
    --cert=/etc/kubernetes/pki/apiserver-etcd-client.crt \
    --key=/etc/kubernetes/pki/apiserver-etcd-client.key
  ```

* **drain이 완료 안 됨 (Terminating 파드 stuck):**
  ```bash
  # Terminating 파드 강제 삭제
  kubectl delete pod <pod-name> -n <ns> --grace-period=0 --force

  # drain --timeout 옵션으로 최대 대기 시간 설정
  kubectl drain worker-1 --ignore-daemonsets --timeout=5m
  ```

* **업그레이드 롤백 (etcd 복구):**
  ```bash
  # kubeadm 업그레이드는 공식 롤백 없음 → etcd 스냅샷으로 복구
  systemctl stop kubelet

  ETCDCTL_API=3 etcdctl snapshot restore /backup/pre-upgrade-etcd.db \
    --data-dir=/var/lib/etcd-restore

  # /etc/kubernetes/manifests/etcd.yaml에서 data-dir 변경
  # --data-dir=/var/lib/etcd-restore

  systemctl start kubelet
  ```
