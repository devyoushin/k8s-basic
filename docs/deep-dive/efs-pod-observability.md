# EFS 접근 Pod 및 I/O 관측

## 1. 개요 및 비유

Amazon EFS(Elastic File System)를 사용하는 파드(Pod)를 찾는 일은 건물의 입주자 명부를 확인하는 일이고, EFS I/O를 확인하는 일은 건물 전체 수도 계량기를 보는 일에 가까움.

Kubernetes 리소스의 PV(PersistentVolume) → PVC(PersistentVolumeClaim) → Pod 연결로 **어떤 Pod가 EFS를 마운트했는지** 정확히 확인함. 반면 AWS/EFS CloudWatch 지표는 파일시스템 단위이며, VPC Flow Logs는 ENI 단위 네트워크 흐름임. 따라서 기본 기능만으로는 **Pod별 EFS 읽기/쓰기 바이트와 NFS 요청 수를 분리할 수 없음**.

관측 범위:

| 질문 | 기본 도구 | 식별 단위 | 답변 가능 여부 |
|---|---|---|---|
| 어떤 Pod가 EFS를 사용하는가 | Kubernetes API, `efs-pod-map.sh` | Pod/PVC/PV | 가능 |
| 어느 노드가 EFS에 연결하는가 | EFS `ClientConnections`, VPC Flow Logs | 노드/ENI | 가능 |
| EFS 전체 읽기·쓰기·메타데이터 요청은 얼마인가 | AWS/EFS CloudWatch | 파일시스템 | 가능 |
| 특정 Pod가 보낸 EFS 읽기·쓰기 바이트/요청은 얼마인가 | 기본 EFS, VPC Flow Logs, EKS audit log | Pod | 불가 |
| 누가 PVC 또는 EFS 사용 Pod를 생성·수정했는가 | EKS control plane audit log | 사용자/서비스 계정/API 객체 | 가능 |

---

## 2. 핵심 설명

### 2.1 EFS를 마운트한 Pod 식별

Amazon EFS CSI(Container Storage Interface) 드라이버를 사용하면 PV의 `.spec.csi.driver`는 `efs.csi.aws.com`임. PV의 `claimRef`는 PVC를, Pod의 `.spec.volumes[].persistentVolumeClaim.claimName`은 PVC를 참조함.

```text
EFS 파일시스템 또는 Access Point
             ↓
PV: spec.csi.driver=efs.csi.aws.com
             ↓ claimRef
PVC
             ↓ volumes[].persistentVolumeClaim.claimName
Pod
             ↓ spec.nodeName
워커 노드
```

저장소에 포함된 조회 전용 스크립트 실행:

```bash
bash ops/scripts/efs-pod-map.sh
```

출력 예시:

```text
PV                 NAMESPACE  PVC          POD              NODE          EFS_VOLUME_HANDLE
pvc-0a1b2c3d       payments   shared-data  api-7fb77-8zk9p  ip-10-0-1-10  fs-0123456789abcdef0::fsap-0123456789abcdef0
```

`EFS_VOLUME_HANDLE`의 앞부분 `fs-...`가 EFS 파일시스템 ID임. 동적 프로비저닝에서는 `fs-...::fsap-...` 형식으로 Access Point ID가 함께 표시됨. 정적 PV는 디렉터리 경로를 포함한 다른 형식일 수 있으므로 `volumeHandle` 원문을 보존해 확인함.

수동 확인 명령:

```bash
# EFS CSI PV와 파일시스템/Access Point handle 확인
kubectl get pv \
  -o custom-columns=PV:.metadata.name,CLAIM_NAMESPACE:.spec.claimRef.namespace,CLAIM_NAME:.spec.claimRef.name,DRIVER:.spec.csi.driver,HANDLE:.spec.csi.volumeHandle

# 특정 PVC를 참조하는 Pod 확인
NAMESPACE="<NAMESPACE>"
PVC_NAME="<PVC_NAME>"
kubectl get pods -n "${NAMESPACE}" -o json \
  | jq -r --arg pvc "${PVC_NAME}" '
      .items[]
      | select(any(.spec.volumes[]?; .persistentVolumeClaim.claimName == $pvc))
      | [.metadata.name, .spec.nodeName, .status.podIP] | @tsv'
```

### 2.2 EFS 전체 I/O와 요청 수 확인

CloudWatch의 `AWS/EFS` 지표는 `FileSystemId` 차원만 제공함. 동일 EFS를 여러 Pod가 공유하면 모든 Pod의 I/O가 합산됨.

| 지표 | 통계 | 의미 |
|---|---|---|
| `DataReadIOBytes` | `Sum` | 파일시스템 전체 읽기 바이트 |
| `DataWriteIOBytes` | `Sum` | 파일시스템 전체 쓰기 바이트 |
| `MetadataIOBytes` | `Sum` | 파일시스템 전체 메타데이터 I/O 바이트 |
| 위 세 I/O 지표 | `SampleCount` | 해당 기간의 EFS 처리 작업 수 |
| `TotalIOBytes` | `Sum` | EFS가 처리한 실제 전체 I/O 바이트 |
| `ClientConnections` | `Sum` | EFS 클라이언트 연결 수; 일반 클라이언트에서는 마운트한 EC2 인스턴스 기준 |
| `PercentIOLimit` | `Maximum` | General Purpose 성능 모드의 I/O 한도 근접도 |

AWS CLI로 최근 1시간의 파일시스템 전체 읽기/쓰기 I/O 확인:

```bash
EFS_ID="fs-0123456789abcdef0"
START_TIME=$(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ)
END_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

for metric in DataReadIOBytes DataWriteIOBytes MetadataIOBytes; do
  aws cloudwatch get-metric-statistics \
    --namespace AWS/EFS \
    --metric-name "${metric}" \
    --dimensions Name=FileSystemId,Value="${EFS_ID}" \
    --statistics Sum SampleCount \
    --period 300 \
    --start-time "${START_TIME}" \
    --end-time "${END_TIME}" \
    --output table
done
```

`Sum`을 300초로 나누면 해당 5분 구간의 평균 바이트/초, `SampleCount`를 300초로 나누면 평균 작업/초를 계산함. 이 값은 Pod별 값이 아니라 동일 파일시스템을 쓰는 모든 클라이언트의 합계임.

### 2.3 VPC Flow Logs로 EFS 연결 노드와 전송량 좁히기

EFS CSI 드라이버는 워커 노드에서 EFS를 NFSv4.1/TCP 2049로 마운트함. Pod의 파일 I/O는 노드 커널의 NFS 클라이언트를 통과하므로, Flow Log의 source IP는 보통 Pod IP가 아니라 **워커 노드 ENI의 IP**로 보임.

따라서 VPC Flow Logs는 다음 용도로 사용함.

| 확인 대상 | 가능 여부 | 방법 |
|---|---|---|
| EFS에 연결한 워커 노드 | 가능 | `dstaddr=EFS mount target IP`, `dstport=2049`로 조회 |
| 노드별 NFS 전송 바이트·패킷 | 가능 | Flow Log의 `bytes`, `packets`를 노드 ENI/IP별 합산 |
| Pod별 NFS 전송 바이트·요청 수 | 불가 | 노드가 여러 Pod의 NFS I/O를 하나의 연결로 집계 |
| NFS 작업 종류(read/write/metadata) | 불가 | Flow Log는 L3/L4 흐름 로그이며 NFS RPC를 해석하지 않음 |

먼저 EFS mount target IP와 ENI를 확인함.

```bash
EFS_ID="fs-0123456789abcdef0"

aws efs describe-mount-targets \
  --file-system-id "${EFS_ID}" \
  --query 'MountTargets[*].[MountTargetId,AvailabilityZoneName,IpAddress,NetworkInterfaceId]' \
  --output table
```

VPC 또는 워커 노드 ENI에 Flow Log를 활성화하고, CloudWatch Logs Insights에서 기본 Flow Log 형식 기준으로 조회함. mount target IP가 여러 개면 `dstaddr in [...]` 조건에 모두 넣음.

| Flow Log 설정 | 권장값 | 이유 |
|---|---|---|
| 대상 | 워커 노드 ENI 또는 워커 노드 서브넷 | source 노드/ENI를 함께 확인 |
| 트래픽 유형 | `ALL` | 거부된 NFS 연결도 함께 조사 |
| 대상 저장소 | CloudWatch Logs 또는 S3 | CloudWatch Logs Insights 또는 Athena로 집계 |
| 집계 간격 | 1분 | 짧은 배치 작업의 시간 상관관계 확보 |
| 필수 필드 | `interface-id`, `srcaddr`, `dstaddr`, `srcport`, `dstport`, `packets`, `bytes`, `action` | 노드별 NFS 흐름과 전송량 집계 |

```text
fields @timestamp, @message
| parse @message "* * * * * * * * * * * * * *" as version, account_id, interface_id, srcaddr, dstaddr, srcport, dstport, protocol, packets, bytes, start, end, action, log_status
| filter action = "ACCEPT" and dstport = 2049 and dstaddr = "<MOUNT_TARGET_IP>"
| stats sum(bytes) as transferred_bytes, sum(packets) as packets, count(*) as flow_records by srcaddr, interface_id
| sort transferred_bytes desc
```

이 결과의 `srcaddr` 또는 `interface_id`를 Kubernetes Node의 `InternalIP`와 대조함. 해당 노드에 스케줄된 EFS Pod는 `efs-pod-map.sh` 결과로 확인함. 이 결합은 **후보 Pod를 좁히는 분석**이며, 노드에 EFS Pod가 여러 개면 단일 Pod의 I/O로 확정할 수 없음.

### 2.4 EKS control plane audit log로 생성·수정 주체 확인

EKS audit log는 Kubernetes API 요청을 기록함. 즉 PVC, PV, Pod를 누가 생성·수정·삭제했는지는 확인하지만, 컨테이너가 EFS 파일을 `read(2)` 또는 `write(2)`한 데이터 경로 이벤트는 기록하지 않음.

Audit 로그를 활성화함. 기존 설정을 확인한 뒤 필요한 유형만 활성화함.

```bash
CLUSTER_NAME="<EKS_CLUSTER_NAME>"
REGION="ap-northeast-2"

aws eks describe-cluster \
  --name "${CLUSTER_NAME}" \
  --region "${REGION}" \
  --query 'cluster.logging.clusterLogging' \
  --output table

aws eks update-cluster-config \
  --name "${CLUSTER_NAME}" \
  --region "${REGION}" \
  --logging '{"clusterLogging":[{"types":["audit"],"enabled":true}]}'
```

CloudWatch Logs 그룹 `/aws/eks/<EKS_CLUSTER_NAME>/cluster`에서 다음 Logs Insights 쿼리로 PVC·PV·Pod 변경 이벤트를 찾음.

```text
fields @timestamp, @message
| filter @message like /"resource":"persistentvolumeclaims"/
    or @message like /"resource":"persistentvolumes"/
    or @message like /"resource":"pods"/
| filter @message like /"verb":"create"/
    or @message like /"verb":"patch"/
    or @message like /"verb":"update"/
    or @message like /"verb":"delete"/
| sort @timestamp desc
| limit 200
```

특정 PVC를 조사할 때는 이름 조건을 추가함.

```text
fields @timestamp, @message
| filter @message like /"resource":"persistentvolumeclaims"/
    and @message like /"name":"<PVC_NAME>"/
| sort @timestamp desc
```

Control plane 로그는 활성화 이후 이벤트만 수집하며 CloudWatch Logs 수집·보관·쿼리 비용이 발생함. 분석 목적에 맞게 `audit`부터 켜고 보존 기간을 설정함.

### 2.5 Pod별 사용량이 반드시 필요할 때의 설계

Pod별 사용량을 정확히 과금·성능 분석해야 하면 네트워크 로그에 의존하지 않고 관측 경계를 설계에 넣어야 함.

| 요구 수준 | 권장 방식 | 결과 |
|---|---|---|
| Pod 목록과 논리 격리 | 워크로드별 PVC 및 EFS Access Point 분리 | 어떤 워크로드가 어떤 경로를 쓰는지 확인 |
| 워크로드별 EFS 총 I/O | 워크로드별 EFS 파일시스템 분리 | AWS/EFS 지표를 워크로드 단위로 확인 |
| Pod별 읽기/쓰기 바이트 | 애플리케이션 Prometheus 메트릭에 `namespace`, `pod`, `pvc` 레이블 추가 | 애플리케이션 관점의 정확한 바이트·작업 수 |
| Pod별 시스템콜/NFS 분석 | eBPF 기반 런타임 관측을 도입하고 cgroup→Pod 매핑 보존 | 노드에서 발생한 I/O를 프로세스/Pod 단위로 분석 |

Access Point를 Pod별로 나눠도 AWS/EFS 기본 지표는 Access Point 차원을 제공하지 않음. Access Point 분리는 권한과 경로 격리에는 유효하지만, CloudWatch만으로 Pod별 I/O를 분리하는 방법은 아님.

---

## 3. 트러블슈팅

### EFS CSI PV가 보이는데 스크립트 결과에 Pod가 없음

**증상**: `efs-pod-map.sh`에 PV/PVC가 출력되지 않거나 Pod 열이 비어 있음.

**원인**: PVC가 아직 Pod에 연결되지 않았거나, Pod가 삭제됐거나, EFS를 CSI PV가 아닌 직접 NFS 볼륨으로 마운트했기 때문임.

**해결 방법**:

```bash
kubectl get pv
kubectl get pvc -A
kubectl get pods -A -o wide
kubectl get pods -A -o yaml | rg -n 'persistentVolumeClaim:|server:|nfs:'
```

직접 NFS 볼륨(`spec.volumes[].nfs`)을 쓰는 Pod는 PV→PVC 체인에 없으므로 Pod 매니페스트의 `server`가 대상 EFS DNS 이름인지 별도로 확인함.

### VPC Flow Logs에서 EFS 트래픽이 Pod IP로 보이지 않음

**증상**: port 2049 트래픽의 source IP가 Pod IP가 아닌 노드 IP임.

**원인**: EFS CSI의 NFS 마운트와 데이터 전송은 워커 노드 커널 NFS 클라이언트가 담당함.

**해결 방법**:

```bash
# Flow Log의 source IP와 노드 InternalIP 비교
kubectl get nodes -o wide

# 해당 노드 위의 Pod 확인
kubectl get pods -A -o wide --field-selector spec.nodeName=<NODE_NAME>
```

노드에 여러 EFS Pod가 있으면 Flow Log 바이트를 특정 Pod에 배분하지 않음.

### EFS I/O가 높지만 원인 Pod를 확정할 수 없음

**증상**: `DataReadIOBytes` 또는 `DataWriteIOBytes`가 증가하지만 Pod별 원인이 보이지 않음.

**원인**: EFS CloudWatch 지표의 차원은 `FileSystemId`이며 Pod, PVC, Access Point 차원이 없음.

**해결 방법**: PV→PVC→Pod 매핑과 노드별 Flow Log로 후보를 좁힌 뒤, 다음 배포부터 워크로드별 파일시스템 분리 또는 애플리케이션 메트릭을 적용함.

---

## 4. 모니터링 및 확인

조사 순서:

```text
1. efs-pod-map.sh로 EFS 파일시스템 ID와 Pod/Node 목록 확보
2. AWS/EFS 지표로 같은 시간대의 읽기·쓰기·메타데이터 I/O 증가 확인
3. VPC Flow Logs에서 mount target:2049 기준 노드별 바이트 확인
4. 해당 노드의 EFS Pod와 배치·CronJob 실행 시각 비교
5. audit log로 PVC/Pod 변경 주체와 시점 확인
6. 원인 분리가 계속 필요하면 워크로드별 파일시스템 또는 애플리케이션 메트릭 적용
```

EFS CloudWatch 경보 기준은 워크로드별 기준선으로 설정함. 고정된 바이트 임계값 하나보다 `PercentIOLimit` 상승, 평소 대비 `DataWriteIOBytes` 급증, `ClientConnections` 급증을 함께 알림 조건으로 사용함.

---

## 5. TIP

- EFS는 여러 Pod가 동시에 공유하는 RWX(ReadWriteMany) 저장소이므로, Pod별 책임 분리가 필요하면 디렉터리 권한만으로 끝내지 말고 PVC와 Access Point를 워크로드별로 분리함
- `ClientConnections`는 Pod 수가 아니라 일반적으로 EFS를 마운트한 EC2 노드 수에 가까움
- Flow Log의 `bytes`는 NFS 네트워크 전송량이며 EFS CloudWatch의 I/O 바이트와 일치하지 않을 수 있음. 프로토콜 오버헤드, 메타데이터, 집계 시간 차이가 존재함
- EKS audit log에는 데이터 읽기·쓰기가 기록되지 않음. 파일 접근 감사가 규정상 필요하면 애플리케이션 감사 로그 또는 파일 감사 체계를 별도로 설계함
- 참고: [EKS에서 EFS CSI 드라이버 사용](https://docs.aws.amazon.com/eks/latest/userguide/efs-csi.html), [Amazon EFS CloudWatch 지표](https://docs.aws.amazon.com/efs/latest/ug/efs-metrics.html), [EKS control plane 로그](https://docs.aws.amazon.com/eks/latest/userguide/control-plane-logs.html), [VPC Flow Log 레코드](https://docs.aws.amazon.com/vpc/latest/userguide/flow-log-records.html)
