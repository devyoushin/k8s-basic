## 1. 개요 및 비유
**etcd 심화**는 쿠버네티스의 두뇌 역할을 하는 etcd의 내부 Key-Value 구조, Raft 합의 알고리즘, 고가용성 운영 원리를 다루는 심층 가이드입니다.

💡 **비유하자면 '다수결로 운영되는 분산 공증 사무소'와 같습니다.**
3~5개 공증소(etcd 노드)가 동시에 운영되며, 어떤 서류(데이터)도 과반수 공증소의 도장(쿼럼 동의)을 받아야만 공식 효력이 생깁니다. 한 공증소가 불에 타도(노드 장애) 나머지 공증소의 장부로 클러스터를 계속 운영할 수 있습니다.

---

## 2. 핵심 설명

### 1) etcd 내부 Key-Value 구조

etcd는 단순한 Key-Value 저장소처럼 보이지만 **계층적 Key 경로**를 사용합니다.

```
/registry/
├── pods/
│   ├── default/
│   │   ├── my-pod-abc12        → Pod 오브젝트 (protobuf 직렬화)
│   │   └── my-pod-def34
│   └── kube-system/
│       └── coredns-xyz99
├── deployments/apps/
│   └── default/
│       └── my-deployment
├── services/specs/
│   └── default/
│       └── kubernetes           → kubernetes 서비스 오브젝트
├── secrets/
│   └── default/
│       └── my-secret           → 암호화된 Secret 데이터
├── configmaps/
│   └── kube-system/
│       └── kubeadm-config
├── nodes/
│   └── worker-1                → Node 오브젝트 (노드 상태, 레이블 등)
├── events/
│   └── default/
│       └── my-pod.abc123       → Event 오브젝트 (TTL 존재)
└── leases/
    └── kube-node-lease/
        └── worker-1            → 노드 하트비트 리스
```

**Key 명명 규칙:**
```
/registry/{resource-type}/{api-group}/{namespace}/{name}

예시:
/registry/pods/default/nginx-pod           # core API (group 생략)
/registry/deployments/apps/default/nginx   # apps API group
/registry/ingresses/networking.k8s.io/default/my-ingress
```

**Value 저장 형식:**
- 기본: **Protocol Buffers(protobuf)** 바이너리 직렬화 (JSON보다 ~6배 빠른 인코딩)
- Audit 목적으로 읽을 때는 JSON으로 변환 필요

**Revision(리비전) 시스템:**
```
모든 Key-Value 변경은 단조 증가하는 전역 Revision 번호를 가짐

create_revision: 처음 생성된 Revision
mod_revision:    마지막으로 수정된 Revision
version:         해당 Key의 변경 횟수 (삭제 후 재생성 시 1로 리셋)
```

### 2) Raft 합의 알고리즘

Raft는 분산 시스템에서 **데이터 일관성을 보장**하기 위한 합의(consensus) 프로토콜입니다.

**노드 역할:**

| 역할 | 설명 |
|---|---|
| **Leader** | 모든 쓰기 요청 처리, 팔로워에게 로그 복제 |
| **Follower** | 리더의 로그를 수동적으로 복제, 읽기 요청 처리 가능 |
| **Candidate** | 리더 선출 과정 중 임시 상태 |

**Raft 쓰기 흐름:**

```
1. 클라이언트(API Server) → Leader에 쓰기 요청
         │
2. Leader → 로그 엔트리 생성 (아직 커밋 안 됨)
         │
3. Leader → 모든 Follower에게 AppendEntries RPC 전송
         │
4. Follower들 → 로그 기록 후 ACK 응답
         │
5. Leader → 과반수(Quorum) ACK 수신 시 커밋
         │
6. Leader → 클라이언트에 성공 응답
         │
7. Leader → 다음 AppendEntries에서 Follower들에게 커밋 알림
```

**Quorum(쿼럼) 공식:**
```
Quorum = (N / 2) + 1   (N = 전체 etcd 노드 수, 소수점 버림)

노드 3개 → Quorum = 2 (장애 허용: 1개)
노드 5개 → Quorum = 3 (장애 허용: 2개)
노드 7개 → Quorum = 4 (장애 허용: 3개)

⚠️ 짝수 노드는 비권장: 4개 노드는 Quorum = 3이지만 장애 허용은 1개뿐 (3개와 동일)
```

**리더 선출 (Leader Election):**

```
[정상 상태]
Leader ────heartbeat────→ Follower들 (election timeout 리셋)

[Leader 장애]
Leader 침묵
Follower: election timeout 경과 (150~300ms 랜덤)
    │
    ↓ Candidate로 전환
    │
Candidate: 자신의 Term 증가 → 모든 노드에 RequestVote RPC
    │
    ↓ 과반수 투표 획득 시
    │
New Leader: 즉시 heartbeat 전송으로 권위 확립
```

**Term(임기) 시스템:**
```
Term은 단조 증가하는 논리 시계 (epoch와 유사)
각 선거마다 Term 증가
오래된 Term의 메시지는 자동 무시 → 스플릿 브레인 방지
```

**WAL (Write-Ahead Log):**
```
모든 변경사항을 실제 상태 DB에 적용하기 전에 WAL에 먼저 기록
→ 장애 후 재시작 시 WAL 재생으로 상태 복구

WAL 파일 위치: /var/lib/etcd/member/wal/
Snapshot 파일: /var/lib/etcd/member/snap/
```

**Snapshot 압축:**
```
WAL이 무한정 쌓이면 재시작 시간이 길어짐
→ 주기적으로 현재 상태를 Snapshot으로 저장하고 이전 WAL 정리

etcd 설정:
  --snapshot-count=10000    # 10000번 변경마다 스냅샷 생성
  --auto-compaction-mode=periodic
  --auto-compaction-retention=1h  # 1시간 이전 리비전 자동 압축
```

### 3) etcd 고가용성 아키텍처

```
[쿠버네티스 HA 구성 예시: etcd 3노드]

Control Plane 1          Control Plane 2          Control Plane 3
┌─────────────┐         ┌─────────────┐         ┌─────────────┐
│ API Server  │         │ API Server  │         │ API Server  │
│ etcd Leader │◄───────►│etcd Follower│◄───────►│etcd Follower│
└─────────────┘  Raft   └─────────────┘  Raft   └─────────────┘
       ▲
       │ 2379 (클라이언트)
       │ 2380 (피어 통신)
  [API Server]
```

**Stacked etcd vs External etcd:**

| 구성 | 설명 | 특징 |
|---|---|---|
| Stacked | 컨트롤 플레인 노드에 etcd 내장 | 구성 단순, 리소스 공유 |
| External | 별도 etcd 클러스터 운영 | 격리성, 대규모 클러스터에 적합 |

---

## 3. YAML 적용 예시 (etcdctl 운영 명령어)

### etcd 상태 및 데이터 조회
```bash
# etcd 클러스터 멤버 목록 및 상태
ETCDCTL_API=3 etcdctl \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key \
  member list -w table

# 현재 리더 확인
etcdctl endpoint status --cluster -w table

# 특정 Key 조회 (디버깅)
etcdctl get /registry/pods/default --prefix --keys-only | head -20

# 특정 파드 데이터 조회 (protobuf → JSON 변환 필요)
etcdctl get /registry/pods/default/my-pod | ETCDCTL_API=3 etcdctl ... | \
  kubectl-convert -f - --output-version v1
```

### 백업 및 복구
```bash
# 스냅샷 백업
ETCDCTL_API=3 etcdctl \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key \
  snapshot save /backup/etcd-$(date +%Y%m%d-%H%M%S).db

# 스냅샷 상태 검증
etcdctl snapshot status /backup/etcd-20240101-120000.db -w table
# 출력: hash, revision, total keys, total size

# 스냅샷 복구 (모든 etcd 노드 중지 후 실행)
ETCDCTL_API=3 etcdctl snapshot restore /backup/etcd-20240101-120000.db \
  --name etcd-node-1 \
  --initial-cluster "etcd-node-1=https://192.168.1.1:2380,etcd-node-2=https://192.168.1.2:2380,etcd-node-3=https://192.168.1.3:2380" \
  --initial-cluster-token etcd-cluster-1 \
  --initial-advertise-peer-urls https://192.168.1.1:2380 \
  --data-dir /var/lib/etcd-restored
```

### 데이터 압축 및 조각 모음
```bash
# 현재 Revision 확인
rev=$(etcdctl endpoint status --write-out="json" | jq '.[] | .Status.header.revision')

# 오래된 Revision 압축 (디스크 공간 확보)
etcdctl compact $rev

# 조각 모음 (실제 디스크 공간 반환)
etcdctl defrag --cluster

# 용량 알람 초기화 (용량 초과로 etcd가 읽기 전용 모드 진입 시)
etcdctl alarm disarm
```

### etcd 성능 벤치마크
```bash
# 쓰기 성능 테스트 (디스크 I/O 검증)
etcdctl check perf --load="l"
# 결과: Total requests, Failed requests, Total duration, Throughput, Slowest/Fastest

# 네트워크 레이턴시 확인 (etcd 노드 간)
etcdctl check perf --endpoints=https://etcd-node-2:2379
```

### etcd 정적 파드 설정 확인 (kubeadm 환경)
```yaml
# /etc/kubernetes/manifests/etcd.yaml (핵심 설정만)
apiVersion: v1
kind: Pod
metadata:
  name: etcd
  namespace: kube-system
spec:
  containers:
  - name: etcd
    image: registry.k8s.io/etcd:3.5.x
    command:
    - etcd
    - --data-dir=/var/lib/etcd            # 데이터 저장 경로
    - --listen-client-urls=https://0.0.0.0:2379
    - --advertise-client-urls=https://192.168.1.1:2379
    - --listen-peer-urls=https://0.0.0.0:2380   # 피어(노드 간) 통신 포트
    - --initial-cluster=etcd1=https://192.168.1.1:2380,etcd2=...
    - --snapshot-count=10000              # 스냅샷 빈도
    - --heartbeat-interval=100           # 리더→팔로워 heartbeat 간격 (ms)
    - --election-timeout=1000            # 리더 선출 타임아웃 (heartbeat × 10 권장)
    - --quota-backend-bytes=8589934592   # DB 최대 크기 (8GB)
    # TLS 설정
    - --cert-file=/etc/kubernetes/pki/etcd/server.crt
    - --key-file=/etc/kubernetes/pki/etcd/server.key
    - --peer-cert-file=/etc/kubernetes/pki/etcd/peer.crt
    - --peer-trusted-ca-file=/etc/kubernetes/pki/etcd/ca.crt
    # 자동 압축 (Revision 누적 방지)
    - --auto-compaction-mode=periodic
    - --auto-compaction-retention=8h
```

---

## 4. 트러블 슈팅

* **etcd DB 용량 초과 (`etcdserver: mvcc: database space exceeded`):**
  ```bash
  # 현재 DB 크기 확인
  etcdctl endpoint status -w table   # DB SIZE 컬럼 확인

  # 즉시 조치: 압축 + 조각 모음
  rev=$(etcdctl endpoint status --write-out="json" | jq -r '.[] | .Status.header.revision')
  etcdctl compact $rev
  etcdctl defrag --cluster
  etcdctl alarm disarm

  # 근본 대책: --quota-backend-bytes 증가 (기본 2GB → 최대 8GB)
  ```

* **etcd 리더 선출 반복 (`leader changed`):**
  * 디스크 I/O 레이턴시가 `election-timeout`을 초과하는 경우 발생합니다.
  * `etcdctl endpoint status`에서 `Is Leader` 컬럼이 계속 바뀌는지 확인합니다.
  * 조치: SSD로 교체하거나, `--heartbeat-interval`과 `--election-timeout`을 늘리세요 (예: 250ms/2500ms).
  * `fio` 도구로 디스크 fsync 레이턴시를 측정합니다: `fio --rw=write --ioengine=sync --fdatasync=1 --filename=test --size=22m --bs=2300`

* **etcd 스플릿 브레인 발생:**
  * 네트워크 파티션으로 두 그룹이 각각 Quorum을 달성한 상태입니다.
  * Raft는 단일 Term에서 단 하나의 리더만 허용하므로 이론상 불가능하지만, 네트워크 복구 후 Term이 낮은 리더는 자동으로 Follower로 강등됩니다.
  * 복구 후 `etcdctl member list`로 모든 멤버 상태 확인합니다.

* **특정 쿠버네티스 리소스가 etcd에서 손상된 경우:**
  ```bash
  # 특정 Key 삭제 (매우 주의: API Server 통해 삭제 권장)
  etcdctl del /registry/pods/default/corrupted-pod

  # 백업에서 특정 Key만 복원 (고급)
  # 별도 etcd 인스턴스에 백업 복원 후 해당 Key 추출
  ```

* **etcd 노드 교체 (멤버 교체 절차):**
  ```bash
  # 1. 장애 노드를 멤버 목록에서 제거
  etcdctl member remove <member-id>

  # 2. 새 노드를 learner로 추가 (데이터 동기화 후 투표권 부여)
  etcdctl member add etcd-new --peer-urls=https://new-node:2380 --learner

  # 3. 새 노드에서 etcd 시작 (--initial-cluster-state=existing)

  # 4. learner가 동기화 완료되면 voting member로 승격
  etcdctl member promote <learner-member-id>
  ```

* **etcd 메트릭 모니터링 핵심 지표:**
  ```
  etcd_server_leader_changes_seen_total   → 리더 변경 횟수 (많으면 불안정)
  etcd_disk_wal_fsync_duration_seconds    → WAL 쓰기 레이턴시 (p99 < 10ms 권장)
  etcd_disk_backend_commit_duration_seconds → DB 커밋 레이턴시 (p99 < 25ms 권장)
  etcd_network_peer_round_trip_time_seconds → 피어 간 RTT (< 10ms 권장)
  etcd_mvcc_db_total_size_in_bytes        → DB 크기 (quota의 80% 이하 유지)
  etcd_server_proposals_failed_total      → 합의 실패 횟수 (0이어야 정상)
  ```
