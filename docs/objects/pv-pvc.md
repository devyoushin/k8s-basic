## 1. 개요 및 비유
**PersistentVolume(PV)**은 관리자가 미리 프로비저닝하거나 StorageClass를 통해 동적으로 생성된 스토리지 공간입니다. **PersistentVolumeClaim(PVC)**은 파드가 스토리지를 "청구"하는 요청서입니다.

💡 **비유하자면 'PV는 주차 공간, PVC는 주차 예약권'과 같습니다.**
주차장 관리자(관리자/StorageClass)가 1번~100번 주차 공간(PV)을 만들어 놓습니다. 운전자(파드)는 "SUV 한 대 댈 공간(20Gi, ReadWriteOnce) 주세요"라고 예약권(PVC)을 신청하면, 시스템이 조건에 맞는 빈 주차 공간을 배정해줍니다.

## 2. 핵심 설명
* **바인딩 과정:** PVC가 생성되면 쿠버네티스가 조건(용량, AccessMode, StorageClass)에 맞는 PV를 찾아 1:1로 바인딩합니다.
* **AccessMode(접근 모드):**

| 모드 | 약어 | 설명 |
|---|---|---|
| `ReadWriteOnce` | RWO | 단일 노드에서 읽기/쓰기 (가장 일반적) |
| `ReadOnlyMany` | ROX | 여러 노드에서 읽기 전용 |
| `ReadWriteMany` | RWX | 여러 노드에서 읽기/쓰기 (NFS, EFS 등 필요) |

* **ReclaimPolicy(반환 정책):** PVC 삭제 후 PV의 데이터를 어떻게 처리할지 결정합니다.
  * `Retain`: 데이터 보존 (수동 정리 필요)
  * `Delete`: PV와 실제 스토리지(EBS 볼륨 등)까지 자동 삭제
  * `Recycle`: 데이터 초기화 후 재사용 (deprecated)
* **StorageClass와 동적 프로비저닝:** StorageClass를 사용하면 PVC 생성 시 PV를 자동으로 만들어줍니다. 직접 PV를 만들 필요가 없어집니다.

## 3. YAML 적용 예시

### StorageClass (동적 프로비저닝 설정 - AWS EBS)
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: fast-ssd
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
  iops: "3000"
  throughput: "125"
reclaimPolicy: Delete      # PVC 삭제 시 EBS 볼륨도 삭제
volumeBindingMode: WaitForFirstConsumer  # 파드가 스케줄된 AZ에 볼륨 생성
```

### PersistentVolumeClaim (스토리지 요청)
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: mysql-data
  namespace: default
spec:
  accessModes:
  - ReadWriteOnce
  storageClassName: fast-ssd  # 위 StorageClass 이름
  resources:
    requests:
      storage: 20Gi
```

### 파드에서 PVC 사용
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: mysql
spec:
  containers:
  - name: mysql
    image: mysql:8.0
    volumeMounts:
    - name: data
      mountPath: /var/lib/mysql
  volumes:
  - name: data
    persistentVolumeClaim:
      claimName: mysql-data  # 위 PVC 이름
```

**유용한 명령어:**
```bash
# PV/PVC 상태 확인
kubectl get pv
kubectl get pvc

# PVC가 어느 PV에 바인딩됐는지 확인
kubectl describe pvc mysql-data
```

## 4. 트러블 슈팅
* **PVC가 `Pending` 상태에서 안 넘어옴:**
  * `kubectl describe pvc <이름>` 에서 이벤트를 확인하세요.
  * `storageClassName`이 잘못됐거나, 해당 AZ에 용량이 없거나, CSI 드라이버가 정상 동작하지 않는 경우입니다.
  * `WaitForFirstConsumer` 바인딩 모드면 파드가 스케줄되기 전까지 Pending이 정상입니다.
* **파드가 다른 AZ로 재스케줄됐는데 볼륨 마운트 실패:**
  * EBS 같은 AZ-지역 볼륨은 파드가 생성된 AZ 내에서만 마운트 가능합니다. `WaitForFirstConsumer` 모드를 사용하고, 노드 어피니티나 토폴로지 스프레드를 적절히 설정해야 합니다.
* **PVC 삭제가 안 되고 계속 `Terminating`:**
  * PVC를 사용 중인 파드가 있으면 삭제되지 않습니다. 해당 파드를 먼저 삭제하세요.
