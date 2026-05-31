## 1. 개요 및 비유

CSI(Container Storage Interface)는 Kubernetes가 어떤 스토리지 벤더의 볼륨도 동일한 방식으로 사용할 수 있도록 하는 표준 인터페이스입니다.

💡 **비유하자면 USB 규격과 같습니다.**
어떤 USB 드라이브(CSI 드라이버)를 꽂아도 운영체제(Kubernetes)가 동일하게 인식합니다. 벤더는 CSI 스펙만 구현하면 됩니다.

---

## 2. 스토리지 오브젝트 관계

```
StorageClass (스토리지 종류 정의)
    │
    │ PVC 생성 시 참조
    ▼
PVC (사용 요청)  ←── 파드 spec.volumes에서 참조
    │
    │ 바인딩
    ▼
PV (실제 볼륨 대표 오브젝트)
    │
    │ CSI 드라이버 통해 실제 스토리지에 연결
    ▼
실제 스토리지 (EBS, GCE PD, NFS, Ceph 등)
```

---

## 3. CSI 드라이버 구조

### 3.1 CSI 컴포넌트

```
Kubernetes 클러스터 내 CSI 드라이버 구성:
┌─────────────────────────────────────────────────────────┐
│  CSI Controller (Deployment — 컨트롤 플레인 사이드)      │
│  ├── external-provisioner  : PVC 감지 → 볼륨 생성 요청  │
│  ├── external-attacher     : 볼륨 Attach/Detach 요청    │
│  ├── external-resizer      : 볼륨 크기 조정 요청        │
│  └── CSI Driver (실제 스토리지 API 호출)                 │
└────────────────────┬────────────────────────────────────┘
                     │ gRPC (Unix Socket)
┌────────────────────▼────────────────────────────────────┐
│  CSI Node (DaemonSet — 각 노드에서 실행)                 │
│  ├── node-driver-registrar : kubelet에 CSI 드라이버 등록 │
│  └── CSI Driver            : Mount/Unmount 수행         │
└─────────────────────────────────────────────────────────┘
```

### 3.2 CSI gRPC 인터페이스 (3가지 서비스)

```
IdentityService:
  GetPluginInfo()         → 드라이버 이름, 버전
  GetPluginCapabilities() → 지원 기능 목록

ControllerService (CSI Controller 파드에서):
  CreateVolume()    → 스토리지에 실제 볼륨 생성
  DeleteVolume()    → 볼륨 삭제
  ControllerPublishVolume()   → 볼륨을 노드에 Attach
  ControllerUnpublishVolume() → 볼륨을 노드에서 Detach
  ListVolumes()     → 볼륨 목록
  CreateSnapshot()  → 스냅샷 생성

NodeService (CSI Node 파드에서):
  NodeStageVolume()   → 볼륨을 노드의 글로벌 경로에 마운트 (포맷 포함)
  NodePublishVolume() → 글로벌 경로를 파드 경로로 bind mount
  NodeUnpublishVolume() → 파드 경로 언마운트
  NodeUnstageVolume()   → 노드 글로벌 경로 언마운트
```

---

## 4. PV 동적 프로비저닝 전체 흐름

```
1. 개발자: kubectl apply -f pvc.yaml
   (PVC: 10Gi, storageClass: ebs-gp3)
        │
        ▼
2. API Server: PVC 오브젝트 저장 (status: Pending)
        │
        ▼ Watch 이벤트
3. external-provisioner (CSI Controller 파드 내)
   - PVC에 맞는 StorageClass 찾음
   - CSI Driver의 CreateVolume() gRPC 호출
   - EBS API: CreateVolume(10Gi, gp3, us-east-1a)
   - 반환: volumeId=vol-0abc123
        │
        ▼
4. external-provisioner: PV 오브젝트 생성
   - spec.csi.volumeHandle = vol-0abc123
   - PVC ↔ PV 바인딩 (status: Bound)
        │
        ▼
5. 파드 스케줄링: 파드가 worker-2 노드에 배치됨
        │
        ▼ Watch 이벤트 (VolumeAttachment 오브젝트)
6. external-attacher (CSI Controller 파드 내)
   - ControllerPublishVolume() gRPC 호출
   - EBS API: AttachVolume(vol-0abc123, worker-2-instance-id)
   - VolumeAttachment status: Attached
        │
        ▼
7. kubelet on worker-2
   - CSI Node Driver의 NodeStageVolume() gRPC 호출
   - 볼륨 포맷 (처음인 경우): mkfs.ext4 /dev/xvdf
   - 노드 글로벌 경로에 마운트:
     /var/lib/kubelet/plugins/kubernetes.io/csi/pv/<pv-name>/globalmount
        │
        ▼
8. NodePublishVolume() gRPC 호출
   - 파드 볼륨 경로로 bind mount:
     /var/lib/kubelet/pods/<pod-uid>/volumes/kubernetes.io~csi/<pvc-name>/mount
        │
        ▼
9. 파드 컨테이너에서 볼륨 사용 가능
   /data (컨테이너 내부 경로)
```

```bash
# PVC 바인딩 상태 확인
kubectl get pvc my-pvc -o wide
kubectl describe pvc my-pvc | grep -A5 "Events:"

# VolumeAttachment 오브젝트 확인 (Attach 과정)
kubectl get volumeattachments

# 노드에서 실제 마운트 확인
mount | grep kubernetes
# /dev/xvdf on /var/lib/kubelet/plugins/.../globalmount type ext4
# /var/lib/.../globalmount on /var/lib/kubelet/pods/.../mount type ext4 (bind)

# 파드 내부에서 마운트 확인
kubectl exec my-pod -- df -h /data
kubectl exec my-pod -- mount | grep /data
```

---

## 5. 볼륨 확장 (Volume Resize) 흐름

```
PVC 용량 증가 요청: 10Gi → 20Gi

1. kubectl patch pvc my-pvc --patch '{"spec":{"resources":{"requests":{"storage":"20Gi"}}}}'
        │
        ▼
2. external-resizer: ControllerExpandVolume() gRPC 호출
   - 클라우드 API로 볼륨 크기 확장 (EBS: 10→20Gi)
   - pvc.status.conditions: FileSystemResizePending = True
        │
        ▼
3. kubelet (파드가 해당 PVC 사용 중인 노드):
   NodeExpandVolume() gRPC 호출
   - resize2fs /dev/xvdf (파일시스템 확장)
   - pvc.status.capacity.storage = 20Gi 업데이트
```

```bash
# 볼륨 확장 지원 여부 확인 (StorageClass)
kubectl get storageclass ebs-gp3 -o jsonpath='{.allowVolumeExpansion}'
# true 여야 함

# PVC 용량 변경
kubectl patch pvc my-pvc -p '{"spec":{"resources":{"requests":{"storage":"20Gi"}}}}'

# 확장 진행 상태 확인
kubectl describe pvc my-pvc | grep -A3 "Conditions:"
# Resizing → FileSystemResizePending → 완료
```

---

## 6. 스냅샷 (VolumeSnapshot)

```yaml
# 스냅샷 클래스 정의
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: ebs-snapshot-class
driver: ebs.csi.aws.com
deletionPolicy: Delete

---
# 스냅샷 생성
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: my-snapshot
spec:
  volumeSnapshotClassName: ebs-snapshot-class
  source:
    persistentVolumeClaimName: my-pvc  # 스냅샷할 PVC

---
# 스냅샷에서 새 PVC 복원
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: restored-pvc
spec:
  storageClassName: ebs-gp3
  dataSource:
    name: my-snapshot
    kind: VolumeSnapshot
    apiGroup: snapshot.storage.k8s.io
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 10Gi
```

---

## 7. 트러블슈팅

* **PVC가 Pending 상태에서 멈춤:**
  ```bash
  # 이벤트 확인
  kubectl describe pvc my-pvc

  # StorageClass 존재 확인
  kubectl get storageclass

  # CSI 프로비저너 파드 로그
  kubectl logs -n kube-system -l app=ebs-csi-controller -c csi-provisioner

  # 흔한 원인:
  # - StorageClass 이름 오타
  # - CSI 드라이버 파드 비정상
  # - 클라우드 권한 부족 (IAM 역할)
  # - 용량 부족 (가용 영역 내)
  ```

* **파드가 ContainerCreating에서 멈춤 (볼륨 마운트 실패):**
  ```bash
  kubectl describe pod my-pod | grep -A10 "Events:"
  # "AttachVolume.Attach failed" → Attach 단계 실패
  # "MountVolume.MountDevice failed" → Stage 단계 실패

  # CSI Node 드라이버 로그
  kubectl logs -n kube-system -l app=ebs-csi-node -c ebs-plugin

  # 이전 파드가 볼륨을 들고 있는지 확인 (ReadWriteOnce 볼륨)
  kubectl get volumeattachments | grep <pv-name>
  ```

* **볼륨 확장 후 용량이 안 늘어남:**
  ```bash
  # 파일시스템 리사이즈는 파드가 실행 중이어야 함
  # (NodeExpandVolume은 파드가 있는 노드에서 수행)
  kubectl get pvc my-pvc -o jsonpath='{.status.conditions}'

  # 파드 재시작이 필요한 경우도 있음 (드라이버에 따라)
  kubectl rollout restart deployment my-app
  ```
