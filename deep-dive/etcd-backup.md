# etcd 백업 및 복구

## 1. 개요 및 비유

etcd는 Kubernetes 클러스터의 모든 상태(Pod, Service, Secret 등)를 저장하는 분산 키-값 저장소. etcd 데이터 유실 = 클러스터 전체 상태 유실.

💡 비유: etcd는 회사의 전산 서버 — 이게 날아가면 직원 명단, 계좌 정보, 업무 지시 전부 사라짐. 정기적인 백업 없이는 재해 복구 불가.

---

## 2. 핵심 설명

### 2.1 동작 원리

#### etcd 기본 구성

```
[kube-apiserver]
       │ 모든 상태 읽기/쓰기
       ▼
[etcd cluster]  ← Raft 합의 알고리즘으로 분산 일관성 유지
  ├── etcd-0 (Leader)
  ├── etcd-1 (Follower)
  └── etcd-2 (Follower)
```

- Raft 알고리즘: 리더가 쓰기 처리, 과반수(quorum) 동의 시 커밋
- 최소 3개 노드 권장 (2개 노드 다운 허용 = 5개 노드 클러스터)
- etcd 데이터 디렉토리 기본 경로: `/var/lib/etcd`

#### 백업 방법 비교

| 방법 | 도구 | 특징 |
|------|------|------|
| 스냅샷 | `etcdctl snapshot save` | 공식 권장, 일관성 보장 |
| 디렉토리 복사 | `cp /var/lib/etcd` | etcd 중단 시에만 안전 |
| Velero | Velero + etcd 플러그인 | 애플리케이션 레벨 백업 (etcd 직접 백업 아님) |

#### 백업 주기 권장

| 환경 | 주기 | 보관 기간 |
|------|------|----------|
| 프로덕션 | 1시간 | 30일 |
| 스테이징 | 6시간 | 7일 |
| EKS (관리형) | AWS가 자동 관리 | etcd 직접 접근 불가 |

**EKS 주의**: EKS는 etcd를 AWS가 관리 — 직접 etcd 백업 불필요. 대신 Velero로 워크로드(PV, 매니페스트) 백업.

#### 복구 가능한 시나리오 vs 불가능한 시나리오

| 시나리오 | etcd 백업으로 복구 | 대안 |
|---------|-------------------|------|
| etcd 데이터 디렉토리 삭제 | 가능 | — |
| 실수로 리소스 대량 삭제 | 가능 (스냅샷 시점으로) | — |
| 노드 전체 손실 (etcd 포함) | 가능 | — |
| PersistentVolume 데이터 손실 | 불가 (etcd는 메타데이터만) | Velero + CSI 스냅샷 |
| EKS 컨트롤 플레인 장애 | 불가 (AWS 관리 영역) | AWS SLA에 의존 |

---

### 2.2 YAML 적용 예시

#### 스냅샷 백업 스크립트

```bash
#!/bin/bash
# etcd-backup.sh — etcd 스냅샷 백업

set -euo pipefail

BACKUP_DIR="/backup/etcd"
DATE=$(date +%Y%m%d-%H%M%S)
SNAPSHOT_FILE="${BACKUP_DIR}/etcd-snapshot-${DATE}.db"
RETENTION_DAYS=30

# etcd 연결 정보 (컨트롤 플레인 노드에서 실행)
ETCD_ENDPOINTS="https://127.0.0.1:2379"
ETCD_CACERT="/etc/kubernetes/pki/etcd/ca.crt"
ETCD_CERT="/etc/kubernetes/pki/etcd/server.crt"
ETCD_KEY="/etc/kubernetes/pki/etcd/server.key"

mkdir -p "${BACKUP_DIR}"

# 스냅샷 생성
ETCDCTL_API=3 etcdctl snapshot save "${SNAPSHOT_FILE}" \
  --endpoints="${ETCD_ENDPOINTS}" \
  --cacert="${ETCD_CACERT}" \
  --cert="${ETCD_CERT}" \
  --key="${ETCD_KEY}"

# 스냅샷 무결성 검증
ETCDCTL_API=3 etcdctl snapshot status "${SNAPSHOT_FILE}" \
  --write-out=table

echo "백업 완료: ${SNAPSHOT_FILE}"

# AWS S3 업로드
aws s3 cp "${SNAPSHOT_FILE}" \
  "s3://<BACKUP_BUCKET>/etcd/$(basename ${SNAPSHOT_FILE})" \
  --sse aws:kms \
  --sse-kms-key-id "<KMS_KEY_ARN>"

echo "S3 업로드 완료"

# 오래된 로컬 백업 삭제
find "${BACKUP_DIR}" -name "etcd-snapshot-*.db" \
  -mtime +${RETENTION_DAYS} -delete

echo "오래된 백업 정리 완료 (${RETENTION_DAYS}일 초과)"
```

#### CronJob — 자동 정기 백업 (온프레미스/자체 관리 클러스터)

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: etcd-backup
  namespace: kube-system
spec:
  schedule: "0 * * * *"  # 매시간
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 3
  jobTemplate:
    spec:
      template:
        spec:
          hostNetwork: true  # etcd 로컬 접근을 위해
          nodeSelector:
            node-role.kubernetes.io/control-plane: ""
          tolerations:
            - key: node-role.kubernetes.io/control-plane
              effect: NoSchedule
          containers:
            - name: etcd-backup
              image: bitnami/etcd:3.5
              command: ["/bin/sh", "-c"]
              args:
                - |
                  ETCDCTL_API=3 etcdctl snapshot save /backup/etcd-$(date +%Y%m%d-%H%M%S).db \
                    --endpoints=https://127.0.0.1:2379 \
                    --cacert=/etc/kubernetes/pki/etcd/ca.crt \
                    --cert=/etc/kubernetes/pki/etcd/server.crt \
                    --key=/etc/kubernetes/pki/etcd/server.key && \
                  aws s3 sync /backup/ s3://<BACKUP_BUCKET>/etcd/ \
                    --sse aws:kms \
                    --exclude "*" \
                    --include "etcd-*.db"
              volumeMounts:
                - name: etcd-certs
                  mountPath: /etc/kubernetes/pki/etcd
                  readOnly: true
                - name: backup-dir
                  mountPath: /backup
              env:
                - name: AWS_DEFAULT_REGION
                  value: ap-northeast-2
              resources:
                requests:
                  memory: "128Mi"
                  cpu: "100m"
                limits:
                  memory: "256Mi"
                  cpu: "500m"
              securityContext:
                runAsNonRoot: false  # etcd 인증서 접근을 위해 root 필요
                readOnlyRootFilesystem: false
          volumes:
            - name: etcd-certs
              hostPath:
                path: /etc/kubernetes/pki/etcd
                type: Directory
            - name: backup-dir
              hostPath:
                path: /backup/etcd
                type: DirectoryOrCreate
          restartPolicy: OnFailure
          serviceAccountName: etcd-backup-sa
```

#### Velero — 워크로드 레벨 백업 (EKS 포함)

```bash
# Velero 설치 (AWS S3 백엔드)
velero install \
  --provider aws \
  --plugins velero/velero-plugin-for-aws:v1.9.0 \
  --bucket <BACKUP_BUCKET> \
  --backup-location-config region=ap-northeast-2 \
  --snapshot-location-config region=ap-northeast-2 \
  --secret-file ./credentials-velero \
  --use-node-agent \
  --default-volumes-to-fs-backup
```

```yaml
# 정기 백업 스케줄 (네임스페이스 단위)
apiVersion: velero.io/v1
kind: Schedule
metadata:
  name: payments-backup
  namespace: velero
spec:
  schedule: "0 2 * * *"   # 매일 새벽 2시
  template:
    includedNamespaces:
      - payments
      - auth
    storageLocation: default
    ttl: 720h   # 30일 보관
    snapshotVolumes: true
    defaultVolumesToFsBackup: false  # CSI 스냅샷 사용
```

---

### 2.3 Best Practice

- **백업 무결성 검증 자동화**: 백업 후 반드시 `etcdctl snapshot status`로 검증, 스크립트에 포함
- **S3 + KMS 암호화**: etcd 스냅샷에는 Secret 데이터가 포함 → 저장 시 반드시 암호화
- **복구 훈련 정기 실시**: 백업이 있어도 복구 절차를 모르면 무용지물 → 분기별 복구 드릴(DR Drill) 실시
- **etcd 스냅샷 + Velero 병행**: etcd 백업은 클러스터 상태 복구용, Velero는 특정 워크로드/네임스페이스 단위 복구용으로 역할 분리
- **멀티 AZ etcd**: 프로덕션 etcd 노드를 서로 다른 AZ에 배치 → AZ 장애 시 클러스터 유지

---

## 3. 트러블슈팅

### 3.1 주요 이슈

#### etcd 스냅샷 복구 후 클러스터 정상 기동 안 됨

**증상**: etcd 복구 후 kube-apiserver가 연결하지 못함

**원인**: 복구 시 `--initial-cluster-token` 미일치 또는 데이터 디렉토리 권한 오류

**해결 방법**:
```bash
# 1. 기존 etcd 데이터 백업 후 삭제
mv /var/lib/etcd /var/lib/etcd.bak

# 2. 스냅샷에서 복구 (새 클러스터 토큰 지정)
ETCDCTL_API=3 etcdctl snapshot restore /backup/etcd-snapshot.db \
  --data-dir=/var/lib/etcd \
  --name=etcd-0 \
  --initial-cluster="etcd-0=https://<NODE_IP>:2380" \
  --initial-cluster-token=etcd-cluster-restored-$(date +%s) \
  --initial-advertise-peer-urls=https://<NODE_IP>:2380

# 3. 디렉토리 소유권 복구
chown -R etcd:etcd /var/lib/etcd

# 4. etcd 재시작
systemctl restart etcd

# 5. etcd 상태 확인
ETCDCTL_API=3 etcdctl endpoint health \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key
```

#### etcd 디스크 사용량 폭증 (mvcc: database space exceeded)

**증상**: kube-apiserver가 etcd 쓰기 거부, `mvcc: database space exceeded` 오류

**원인**: etcd 기본 크기 제한(2GB, 최대 8GB) 초과 또는 컴팩션 미실행

**해결 방법**:
```bash
# 1. 컴팩션 (오래된 리비전 정리)
ETCD_REVISION=$(ETCDCTL_API=3 etcdctl endpoint status \
  --write-out=json | jq '.[0].Status.header.revision')

ETCDCTL_API=3 etcdctl compact ${ETCD_REVISION} \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key

# 2. 조각 모음 (디스크 공간 실제 반환)
ETCDCTL_API=3 etcdctl defrag \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key

# 3. DB 크기 제한 상향 (긴급 조치, 루트 원인 해결 병행)
# etcd Pod 또는 systemd 서비스에 추가
# --quota-backend-bytes=8589934592  (8GB)

# 4. 알람 해제 (컴팩션/defrag 후)
ETCDCTL_API=3 etcdctl alarm disarm \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key
```

---

### 3.2 자주 발생하는 문제

#### etcdctl 연결 거부 (인증서 오류)

**증상**: `dial tcp: connection refused` 또는 `certificate signed by unknown authority`

**해결 방법**:
```bash
# 인증서 경로 확인 (kubeadm 기준)
ls -la /etc/kubernetes/pki/etcd/

# 인증서 유효기간 확인
openssl x509 -in /etc/kubernetes/pki/etcd/server.crt -noout -dates

# etcd 리스닝 포트 확인
ss -tlnp | grep 2379
```

#### Velero 백업 후 복구 시 PV 데이터 누락

**증상**: Velero 복구 후 Pod는 기동되지만 데이터 없음

**원인**: CSI 스냅샷 복구 시 StorageClass가 다른 클러스터에 없음

**해결 방법**:
```bash
# 복구 전 StorageClass 확인
velero restore describe <RESTORE_NAME> | grep "StorageClass"

# StorageClass 매핑으로 복구
velero restore create --from-backup <BACKUP_NAME> \
  --namespace-mappings payments:payments-restored \
  --restore-volumes=true
```

---

## 4. 모니터링 및 확인

```bash
# etcd 클러스터 상태 확인
ETCDCTL_API=3 etcdctl endpoint status \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key \
  --write-out=table

# etcd 멤버 목록
ETCDCTL_API=3 etcdctl member list \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key \
  --write-out=table

# etcd DB 크기 확인
ETCDCTL_API=3 etcdctl endpoint status \
  --write-out=json | jq '.[].Status.dbSize'

# 스냅샷 상태 검증
ETCDCTL_API=3 etcdctl snapshot status /backup/etcd-snapshot.db \
  --write-out=table

# Velero 백업 목록
velero backup get

# Velero 백업 상세 (성공/실패 여부)
velero backup describe <BACKUP_NAME> --details

# etcd Prometheus 메트릭 (주요 지표)
# etcd_server_leader_changes_seen_total: 리더 변경 횟수 (잦으면 불안정)
# etcd_disk_wal_fsync_duration_seconds: WAL 쓰기 지연 (10ms 이상이면 경고)
# etcd_mvcc_db_total_size_in_bytes: DB 크기
curl -s http://localhost:2381/metrics | grep -E "etcd_server_leader|etcd_disk_wal|etcd_mvcc_db"
```

---

## 5. TIP

- **etcd 백업 테스트는 별도 노드에서**: 복구 테스트 시 운영 etcd와 같은 노드에서 하면 기존 클러스터 손상 위험 → 임시 VM 또는 테스트 클러스터에서 실시
- **자동 컴팩션 설정**: etcd 시작 옵션에 `--auto-compaction-mode=periodic --auto-compaction-retention=1h` 추가 → 수동 컴팩션 없이 자동 정리
- **etcd 메트릭 모니터링 필수 지표**: `etcd_disk_wal_fsync_duration_seconds_p99` > 10ms이면 디스크 I/O 병목으로 클러스터 불안정 전조 → SSD 디스크 사용 권장
- **kubeadm 클러스터 업그레이드 전 백업**: `kubeadm upgrade apply` 전 반드시 etcd 스냅샷 생성 — 업그레이드 실패 시 유일한 롤백 수단
- **EKS에서 etcd 걱정은 AWS에**: EKS 컨트롤 플레인은 AWS 책임 범위 — 대신 워크로드(PV, ConfigMap, Secret)를 Velero로 백업하는 데 집중
