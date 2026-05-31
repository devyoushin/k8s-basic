## 1. 개요 및 비유

클러스터 백업은 **etcd 상태 백업**과 **영속 볼륨 데이터 백업** 두 축으로 구성됩니다. 재해 복구(DR)는 이 둘을 복원하는 절차입니다.

💡 **비유하자면 도시 설계도(etcd)와 건물 안 물건(PV)의 백업과 같습니다.**
설계도만 복원하면 도시 구조는 되살아나지만, 건물 안 데이터(DB, 파일)는 별도로 복원해야 합니다.

---

## 2. etcd 백업

### 2.1 etcd가 저장하는 것

```
etcd에 저장되는 것 (백업으로 복원 가능):
✅ 모든 Kubernetes 오브젝트 (Pod, Deployment, Service, Secret 등)
✅ RBAC 정책, ConfigMap
✅ PV/PVC 메타데이터 (실제 데이터는 아님)
✅ CustomResource 인스턴스

etcd에 없는 것 (별도 백업 필요):
❌ 영속 볼륨 실제 데이터 (EBS 스냅샷, NFS 데이터 등)
❌ 컨테이너 로그
❌ 이미지 레지스트리 데이터
```

### 2.2 수동 etcd 스냅샷

```bash
# etcd 스냅샷 생성
ETCDCTL_API=3 etcdctl snapshot save /backup/etcd-$(date +%Y%m%d-%H%M).db \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/healthcheck-client.crt \
  --key=/etc/kubernetes/pki/etcd/healthcheck-client.key

# 스냅샷 무결성 검증
ETCDCTL_API=3 etcdctl snapshot status /backup/etcd-20240105-1200.db \
  --write-out=table
# 출력:
# +----------+----------+------------+------------+
# |   HASH   | REVISION | TOTAL KEYS | TOTAL SIZE |
# +----------+----------+------------+------------+
# | 3d2a5c8f |    12345 |       4523 |     5.2 MB |
# +----------+----------+------------+------------+

# 스냅샷에서 복구 (kubelet 중지 후 수행)
systemctl stop kubelet

ETCDCTL_API=3 etcdctl snapshot restore /backup/etcd-20240105-1200.db \
  --name=master-1 \
  --initial-cluster=master-1=https://192.168.1.10:2380 \
  --initial-cluster-token=etcd-cluster-1 \
  --initial-advertise-peer-urls=https://192.168.1.10:2380 \
  --data-dir=/var/lib/etcd-restored

# etcd manifest 업데이트: --data-dir 경로 변경
vi /etc/kubernetes/manifests/etcd.yaml
# hostPath path: /var/lib/etcd → /var/lib/etcd-restored

systemctl start kubelet
```

### 2.3 자동 백업 스케줄 (CronJob)

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: etcd-backup
  namespace: kube-system
spec:
  schedule: "0 */6 * * *"   # 6시간마다
  jobTemplate:
    spec:
      template:
        spec:
          hostNetwork: true
          nodeSelector:
            node-role.kubernetes.io/control-plane: ""
          tolerations:
          - key: node-role.kubernetes.io/control-plane
            effect: NoSchedule
          containers:
          - name: etcd-backup
            image: bitnami/etcd:3.5
            env:
            - name: ETCDCTL_API
              value: "3"
            command:
            - /bin/sh
            - -c
            - |
              BACKUP_FILE="/backup/etcd-$(date +%Y%m%d-%H%M).db"
              etcdctl snapshot save $BACKUP_FILE \
                --endpoints=https://127.0.0.1:2379 \
                --cacert=/etc/kubernetes/pki/etcd/ca.crt \
                --cert=/etc/kubernetes/pki/etcd/healthcheck-client.crt \
                --key=/etc/kubernetes/pki/etcd/healthcheck-client.key
              # 7일 이상 된 백업 삭제
              find /backup -name "etcd-*.db" -mtime +7 -delete
              echo "Backup completed: $BACKUP_FILE"
            volumeMounts:
            - name: etcd-certs
              mountPath: /etc/kubernetes/pki/etcd
              readOnly: true
            - name: backup-storage
              mountPath: /backup
          volumes:
          - name: etcd-certs
            hostPath:
              path: /etc/kubernetes/pki/etcd
          - name: backup-storage
            persistentVolumeClaim:
              claimName: etcd-backup-pvc   # S3/NFS 마운트 PVC
          restartPolicy: OnFailure
```

---

## 3. Velero — 클러스터 전체 백업

Velero는 etcd + PV 데이터를 함께 백업하고 다른 클러스터로 복원할 수 있습니다.

### 3.1 Velero 아키텍처

```
Velero 동작 흐름:
┌─────────────────────────────────────────────────────────┐
│ Backup 요청                                             │
│   1. Velero가 API Server에서 리소스 목록 조회           │
│   2. 각 리소스를 JSON으로 직렬화                        │
│   3. Object Storage(S3, GCS)에 업로드                  │
│   4. PV 데이터: CSI VolumeSnapshot 또는 restic으로 백업 │
└─────────────────────────────────────────────────────────┘

백업 저장소:
Kubernetes 리소스: S3/GCS에 JSON 파일로 저장
PV 데이터:
  방법 1: CSI VolumeSnapshot (스냅샷 지원 드라이버 필요)
  방법 2: File System Backup (restic/kopia) — 파일 직접 복사

```

### 3.2 Velero 설치 및 백업

```bash
# Velero CLI 설치
brew install velero  # macOS
# 또는 https://github.com/vmware-tanzu/velero/releases

# AWS S3 백업 설정으로 설치
velero install \
  --provider aws \
  --plugins velero/velero-plugin-for-aws:v1.8.0 \
  --bucket my-velero-backup \
  --backup-location-config region=ap-northeast-2 \
  --snapshot-location-config region=ap-northeast-2 \
  --secret-file ./credentials-velero \
  --use-node-agent \          # PV 파일 백업 활성화 (restic)
  --default-volumes-to-fs-backup  # 모든 PV 파일 백업

# 전체 클러스터 백업
velero backup create full-backup-$(date +%Y%m%d) \
  --include-namespaces="*" \
  --default-volumes-to-fs-backup

# 특정 네임스페이스만 백업
velero backup create prod-backup \
  --include-namespaces production,staging \
  --ttl 720h  # 30일 보관

# 스케줄 백업 설정
velero schedule create daily-backup \
  --schedule="0 2 * * *" \   # 매일 새벽 2시
  --include-namespaces="*" \
  --ttl 168h                  # 7일 보관
```

### 3.3 Velero 복원

```bash
# 백업 목록 확인
velero backup get
# NAME                   STATUS    ERRORS   WARNINGS   CREATED   EXPIRES
# full-backup-20240105   Completed 0        0          ...       29d

# 백업 내용 확인
velero backup describe full-backup-20240105
velero backup logs full-backup-20240105

# 전체 복원
velero restore create --from-backup full-backup-20240105

# 특정 네임스페이스만 복원
velero restore create \
  --from-backup full-backup-20240105 \
  --include-namespaces production

# 다른 네임스페이스로 복원 (네임스페이스 이름 변경)
velero restore create \
  --from-backup full-backup-20240105 \
  --include-namespaces production \
  --namespace-mappings production:production-restore

# 복원 상태 확인
velero restore get
velero restore describe <restore-name>
```

---

## 4. 재해 복구 시나리오별 대응

### 4.1 단일 노드 장애

```
워커 노드 1대 장애:
  → 자동 감지: NodeNotReady (40초 후)
  → 파드 재배치: 다른 노드에 재스케줄 (기본 5분 후 taint)
  → 대응: 노드 교체 또는 수리 후 rejoin

# 빠른 파드 재배치 (5분 기다리지 않고)
kubectl taint node worker-1 node.kubernetes.io/not-ready:NoExecute
# → tolerationSeconds 초과 파드들이 즉시 다른 노드로 이동

# 노드 복구 후 클러스터 재조인
kubeadm token create --print-join-command
# 새 노드에서 출력된 명령 실행
```

### 4.2 etcd 전체 장애 (클러스터 완전 복구)

```bash
# 모든 컨트롤 플레인 컴포넌트 중지
systemctl stop kubelet
# static pod manifest 임시 이동
mkdir /tmp/manifests-backup
mv /etc/kubernetes/manifests/*.yaml /tmp/manifests-backup/
# (kube-apiserver, kube-controller-manager, kube-scheduler, etcd 파드 중지됨)

# etcd 데이터 디렉토리 초기화
rm -rf /var/lib/etcd

# 스냅샷 복구
ETCDCTL_API=3 etcdctl snapshot restore /backup/etcd-latest.db \
  --name=master-1 \
  --initial-cluster=master-1=https://192.168.1.10:2380 \
  --initial-cluster-token=etcd-cluster-1 \
  --initial-advertise-peer-urls=https://192.168.1.10:2380 \
  --data-dir=/var/lib/etcd

# 권한 설정
chown -R etcd:etcd /var/lib/etcd

# manifest 복원 (컴포넌트 재시작)
mv /tmp/manifests-backup/*.yaml /etc/kubernetes/manifests/
systemctl start kubelet

# 복구 확인 (수분 소요)
kubectl get nodes
kubectl get pods -A
```

### 4.3 멀티 클러스터 DR (Active-Passive)

```
Region A (Active) ────백업────> S3/GCS
                                    │
                              velero restore
                                    │
Region B (Passive) ─────────────────┘
  (DR 발동 시 복구)

RTO/RPO 목표:
  RTO (목표 복구 시간): Velero 복원 + DNS 전환 ≈ 30~60분
  RPO (목표 복구 시점): 마지막 스케줄 백업 시점 (예: 최대 1시간 전)
```

---

## 5. 트러블슈팅

* **Velero 백업이 PartiallyFailed:**
  ```bash
  velero backup describe <backup-name>
  velero backup logs <backup-name> | grep -i error

  # 특정 리소스 제외하고 재백업
  velero backup create retry-backup \
    --from-schedule daily-backup \
    --exclude-resources pods,events   # 문제 리소스 제외
  ```

* **etcd 스냅샷 복구 후 파드가 안 뜸:**
  ```bash
  # 노드 re-register 필요한 경우
  kubectl get nodes  # NotReady면

  # 워커 노드에서 kubelet 재시작
  systemctl restart kubelet

  # 인증서 불일치인 경우 노드 삭제 후 재조인
  kubectl delete node worker-1
  kubeadm token create --print-join-command
  ```

* **Velero PV 백업 실패 (restic 에러):**
  ```bash
  # node-agent DaemonSet 상태 확인
  kubectl get pods -n velero -l name=node-agent

  # 특정 노드의 restic 로그
  kubectl logs -n velero node-agent-<pod> | grep error

  # PVC에 백업 어노테이션 명시적 추가
  kubectl annotate pvc my-pvc \
    backup.velero.io/backup-volumes=my-volume
  ```
