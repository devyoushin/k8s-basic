## 1. 개요 및 비유

Kubernetes Audit Log(감사 로그)는 API Server로 들어온 요청을 시간순으로 기록하는 보안·운영 증적이다. `kubectl`, 컨트롤러, ServiceAccount, 외부 자동화가 어떤 리소스에 어떤 verb로 접근했는지 남김.

💡 **비유하자면 '건물 출입 기록 + CCTV 이벤트 목록'과 같음.**
RBAC(Role-Based Access Control)이 "누가 어느 문을 열 수 있는지 정하는 출입 권한표"라면, Audit Log는 "누가 실제로 어느 문 앞에 왔고, 열었고, 실패했는지 남기는 기록"이다. 권한 설계가 맞는지 검증하고, 사고 발생 시 실제 행위를 역추적하는 근거가 됨.

---

## 2. 핵심 설명

### 2.1 Audit Log가 남기는 정보

Audit Log는 Kubernetes API 요청의 행위자를 중심으로 기록한다.

| 필드 | 의미 | 예시 |
|---|---|---|
| `user.username` | 요청 주체 | `admin@example.com`, `system:serviceaccount:default:app-sa` |
| `user.groups` | 인증된 그룹 | `system:authenticated`, `system:masters` |
| `verb` | API 작업 | `get`, `list`, `create`, `patch`, `delete` |
| `objectRef.resource` | 대상 리소스 | `pods`, `secrets`, `deployments` |
| `objectRef.namespace` | 대상 네임스페이스 | `default`, `kube-system` |
| `requestURI` | 실제 API 경로 | `/api/v1/namespaces/default/secrets/db` |
| `sourceIPs` | 요청 출발 IP | `10.0.1.20` |
| `userAgent` | 요청 클라이언트 | `kubectl/v1.30`, `controller-manager` |
| `responseStatus.code` | 응답 코드 | `200`, `201`, `403`, `404` |

### 2.2 Audit Level

Audit Policy(감사 정책)는 리소스·verb·user 조건에 따라 로그 레벨을 결정한다.

| Level | 기록 범위 | 사용 기준 |
|---|---|---|
| `None` | 기록 안 함 | 고빈도 시스템 read 요청 제외 |
| `Metadata` | 메타데이터만 기록 | 일반 조회, 대부분의 운영 이벤트 |
| `Request` | 메타데이터 + 요청 바디 | 생성·수정 요청의 입력값 확인 |
| `RequestResponse` | 메타데이터 + 요청/응답 바디 | 민감 작업 정밀 감사. 로그 용량·민감정보 노출 주의 |

`RequestResponse`는 강력하지만 Secret 값, 토큰 요청 응답, 리소스 spec이 로그에 남을 수 있음. 민감 데이터가 포함되는 리소스는 보관 위치, 접근 권한, 암호화 정책을 먼저 정해야 함.

### 2.3 Audit Stage

Kubernetes 감사 이벤트는 요청 처리 단계별로 생성된다.

| Stage | 의미 | 운영 판단 |
|---|---|---|
| `RequestReceived` | API Server가 요청을 받음 | 로그량이 많아 보통 생략 |
| `ResponseStarted` | long-running 요청 응답 시작 | watch, exec, port-forward 추적 |
| `ResponseComplete` | 응답 완료 | 일반 API 요청 추적의 기본 |
| `Panic` | 요청 처리 중 panic 발생 | API Server 장애 분석 |

일반 운영 정책은 `omitStages: ["RequestReceived"]`를 사용해 로그량을 줄이고, 완료된 요청 중심으로 본다.

### 2.4 권장 Audit Policy

아래 정책은 kubeadm, Kubespray, self-managed control plane에서 사용 가능한 기본 정책 예시다.

```yaml
apiVersion: audit.k8s.io/v1
kind: Policy
omitStages:
  - RequestReceived
rules:
  # Secret 본문 응답은 기록하지 않고 접근 메타데이터만 남김
  - level: Metadata
    resources:
      - group: ""
        resources:
          - secrets

  # ServiceAccount 토큰 발급 요청은 요청 바디까지 기록
  - level: Request
    verbs:
      - create
    resources:
      - group: ""
        resources:
          - serviceaccounts/token

  # exec, attach, port-forward, proxy는 침해 분석 핵심 이벤트
  - level: Request
    verbs:
      - create
    resources:
      - group: ""
        resources:
          - pods/exec
          - pods/attach
          - pods/portforward
          - pods/proxy
          - services/proxy
          - nodes/proxy

  # RBAC 변경은 누가 권한을 바꿨는지 추적
  - level: Request
    resources:
      - group: rbac.authorization.k8s.io
        resources:
          - roles
          - rolebindings
          - clusterroles
          - clusterrolebindings

  # 워크로드 생성·수정·삭제 요청 추적
  - level: Request
    verbs:
      - create
      - update
      - patch
      - delete
    resources:
      - group: apps
        resources:
          - deployments
          - daemonsets
          - statefulsets
          - replicasets
      - group: batch
        resources:
          - jobs
          - cronjobs
      - group: ""
        resources:
          - pods
          - services
          - configmaps

  # kubelet, controller, scheduler의 고빈도 read 요청은 제외
  - level: None
    users:
      - system:kube-proxy
      - system:kube-scheduler
      - system:kube-controller-manager
    verbs:
      - get
      - list
      - watch

  # events는 양이 많고 보안 증적 가치가 낮아 제외
  - level: None
    resources:
      - group: ""
        resources:
          - events
      - group: events.k8s.io
        resources:
          - events

  # 나머지 요청은 메타데이터 기록
  - level: Metadata
```

### 2.5 kubeadm API Server에 Audit Log 활성화

`/etc/kubernetes/audit-policy.yaml`을 control plane 노드에 배치하고, static Pod로 실행되는 kube-apiserver manifest에 플래그와 hostPath mount를 추가한다.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: kube-apiserver
  namespace: kube-system
spec:
  containers:
    - name: kube-apiserver
      command:
        - kube-apiserver
        - --audit-policy-file=/etc/kubernetes/audit-policy.yaml
        - --audit-log-path=/var/log/kubernetes/audit/audit.log
        - --audit-log-maxage=30
        - --audit-log-maxbackup=10
        - --audit-log-maxsize=100
      volumeMounts:
        - name: audit-policy
          mountPath: /etc/kubernetes/audit-policy.yaml
          readOnly: true
        - name: audit-log
          mountPath: /var/log/kubernetes/audit
          readOnly: false
  volumes:
    - name: audit-policy
      hostPath:
        path: /etc/kubernetes/audit-policy.yaml
        type: File
    - name: audit-log
      hostPath:
        path: /var/log/kubernetes/audit
        type: DirectoryOrCreate
```

운영 클러스터에서는 기존 kube-apiserver manifest 전체를 백업한 뒤 수정한다. static Pod manifest 변경 시 kubelet이 kube-apiserver를 재시작하므로 control plane HA 구성과 API 영향도를 먼저 확인함.

### 2.6 EKS Control Plane Audit Log 활성화

EKS(Elastic Kubernetes Service)는 control plane을 직접 수정하지 않는다. AWS API로 control plane log export를 켜고, CloudWatch Logs에서 `/aws/eks/<CLUSTER_NAME>/cluster` 로그 그룹을 조회한다.

```bash
aws eks update-cluster-config \
  --region ap-northeast-2 \
  --name <CLUSTER_NAME> \
  --logging '{"clusterLogging":[{"types":["api","audit","authenticator","controllerManager","scheduler"],"enabled":true}]}'

aws eks describe-update \
  --region ap-northeast-2 \
  --name <CLUSTER_NAME> \
  --update-id <UPDATE_ID>
```

EKS에서 `audit` 로그는 Kubernetes API 감사 이벤트이고, `authenticator` 로그는 IAM 인증과 Kubernetes username/group 매핑을 확인할 때 사용한다. EKS 권한 장애는 두 로그를 함께 봐야 원인 분리가 쉬움.

### 2.7 감사 대상 우선순위

| 이벤트 | 위험도 | 확인 이유 |
|---|---|---|
| `secrets` 조회 | 높음 | 자격 증명 탈취 가능성 |
| `pods/exec` | 높음 | 컨테이너 내부 명령 실행 |
| `pods/portforward` | 높음 | 네트워크 정책 우회 가능성 |
| `clusterrolebindings` 생성·수정 | 높음 | cluster-admin 권한 상승 |
| `serviceaccounts/token` 생성 | 높음 | 임시 토큰 발급 추적 |
| `deployments` patch/update | 중간 | 이미지 교체, 환경변수 변경 |
| `configmaps` patch/update | 중간 | 설정 변조 |
| `nodes/proxy` | 높음 | kubelet API 경유 접근 |

---

## 3. 트러블슈팅

### Audit Log가 생성되지 않음

**증상**: `/var/log/kubernetes/audit/audit.log` 파일이 없거나 비어 있음

**원인**: `--audit-policy-file`, `--audit-log-path` 플래그 누락, hostPath mount 누락, policy 파일 경로 오류

**해결 방법**:

```bash
sudo grep audit /etc/kubernetes/manifests/kube-apiserver.yaml

sudo ls -l /etc/kubernetes/audit-policy.yaml
sudo ls -ld /var/log/kubernetes/audit

kubectl get pod kube-apiserver-$(hostname) -n kube-system -o yaml | grep audit
```

`kube-apiserver` Pod가 CrashLoopBackOff 상태면 manifest 경로, YAML 들여쓰기, hostPath 타입을 먼저 확인한다.

---

### Audit Log 때문에 디스크 사용량이 급증함

**증상**: control plane 노드의 `/var/log/kubernetes/audit` 사용량 증가, API Server 응답 지연 발생

**원인**: `RequestResponse` 남용, `RequestReceived` stage 기록, watch/list 이벤트 과다, log rotation 미설정

**해결 방법**:

```bash
sudo du -sh /var/log/kubernetes/audit
sudo ls -lh /var/log/kubernetes/audit

sudo grep -E 'audit-log-maxage|audit-log-maxbackup|audit-log-maxsize|audit-log-path' \
  /etc/kubernetes/manifests/kube-apiserver.yaml
```

Audit Policy에서 `omitStages: ["RequestReceived"]`를 적용하고, 고빈도 system user와 `events` 리소스를 `None` 처리한다. `RequestResponse`는 사고 분석이 필요한 리소스에만 제한한다.

---

### Secret 값이 Audit Log에 노출됨

**증상**: audit log에 Secret data, token, 인증 정보가 포함됨

**원인**: `secrets` 리소스에 `RequestResponse` 레벨 적용

**해결 방법**:

```bash
sudo grep -n "secrets" /etc/kubernetes/audit-policy.yaml
sudo grep -n "RequestResponse" /etc/kubernetes/audit-policy.yaml
```

Secret은 기본적으로 `Metadata` 레벨로 기록한다. Secret 본문 감사가 꼭 필요하면 audit log 저장소 암호화, 접근 권한 분리, 짧은 보관 기간을 먼저 적용한다.

---

### EKS에서 audit 로그가 보이지 않음

**증상**: CloudWatch Logs에 `/aws/eks/<CLUSTER_NAME>/cluster` 로그 그룹 또는 `kube-apiserver-audit-*` 스트림이 없음

**원인**: control plane logging 미활성화, logging update 미완료, CloudWatch Logs 권한·리전 착오

**해결 방법**:

```bash
aws eks describe-cluster \
  --region ap-northeast-2 \
  --name <CLUSTER_NAME> \
  --query 'cluster.logging.clusterLogging'

aws logs describe-log-groups \
  --region ap-northeast-2 \
  --log-group-name-prefix /aws/eks/<CLUSTER_NAME>/cluster
```

`audit` 타입이 `enabled: true`인지 확인한다. 로그 그룹은 클러스터가 위치한 리전에서 조회한다.

---

## 4. 모니터링 및 확인

### 4.1 로컬 audit log jq 분석

```bash
# Secret 조회 이력
sudo jq 'select(.objectRef.resource == "secrets" and .verb == "get") |
  {time: .requestReceivedTimestamp, user: .user.username, namespace: .objectRef.namespace, name: .objectRef.name, code: .responseStatus.code}' \
  /var/log/kubernetes/audit/audit.log

# pods/exec 실행 이력
sudo jq 'select(.objectRef.resource == "pods" and .objectRef.subresource == "exec") |
  {time: .requestReceivedTimestamp, user: .user.username, namespace: .objectRef.namespace, pod: .objectRef.name, sourceIPs: .sourceIPs}' \
  /var/log/kubernetes/audit/audit.log

# RBAC 변경 이력
sudo jq 'select(.objectRef.apiGroup == "rbac.authorization.k8s.io" and (.verb == "create" or .verb == "patch" or .verb == "update" or .verb == "delete")) |
  {time: .requestReceivedTimestamp, user: .user.username, verb: .verb, resource: .objectRef.resource, namespace: .objectRef.namespace, name: .objectRef.name}' \
  /var/log/kubernetes/audit/audit.log

# 실패한 요청만 확인
sudo jq 'select(.responseStatus.code >= 400) |
  {time: .requestReceivedTimestamp, user: .user.username, verb: .verb, uri: .requestURI, code: .responseStatus.code, reason: .responseStatus.reason}' \
  /var/log/kubernetes/audit/audit.log
```

### 4.2 EKS CloudWatch Logs Insights 쿼리

```sql
fields @timestamp, user.username, verb, objectRef.resource, objectRef.namespace, objectRef.name, responseStatus.code
| filter objectRef.resource = "secrets" and verb in ["get", "list", "watch"]
| sort @timestamp desc
| limit 50
```

```sql
fields @timestamp, user.username, verb, objectRef.resource, objectRef.subresource, objectRef.namespace, objectRef.name, sourceIPs
| filter objectRef.subresource in ["exec", "portforward", "attach"]
| sort @timestamp desc
| limit 50
```

```sql
fields @timestamp, user.username, verb, objectRef.resource, objectRef.name, responseStatus.code
| filter objectRef.apiGroup = "rbac.authorization.k8s.io"
| filter verb in ["create", "patch", "update", "delete"]
| sort @timestamp desc
| limit 100
```

```sql
fields @timestamp, user.username, verb, requestURI, responseStatus.code, responseStatus.reason
| filter responseStatus.code >= 400
| sort @timestamp desc
| limit 100
```

### 4.3 kube-apiserver audit 메트릭

kube-apiserver는 audit subsystem 상태를 Prometheus metric으로 노출한다.

```bash
kubectl get --raw /metrics | grep 'apiserver_audit'
```

| Metric | 의미 | 확인 기준 |
|---|---|---|
| `apiserver_audit_event_total` | export된 audit event 수 | 급증 시 policy 범위 확인 |
| `apiserver_audit_error_total` | audit export 실패 수 | 0이 아닌 경우 backend·디스크·권한 확인 |

### 4.4 정기 점검 체크리스트

| 주기 | 점검 항목 | 명령/쿼리 |
|---|---|---|
| 매일 | `pods/exec`, `pods/portforward` 사용 이력 | audit jq 또는 Logs Insights |
| 매일 | Secret 조회 실패·성공 이력 | `objectRef.resource="secrets"` |
| 매주 | RBAC 변경 이력 | `rbac.authorization.k8s.io` |
| 매주 | 403 Forbidden 급증 | `responseStatus.code=403` |
| 매월 | Audit Policy 과다/누락 점검 | policy 파일 review |
| 매월 | 로그 보관·접근 권한 점검 | S3/CloudWatch/IAM/노드 파일 권한 |

---

## 5. TIP

- Audit Log는 RBAC을 대체하지 않음. RBAC은 사전 차단, Audit Log는 사후 추적 역할임.
- `RequestResponse`는 최소화함. 특히 Secret, TokenReview, SubjectAccessReview, admission webhook 관련 요청은 민감 정보가 섞일 수 있음.
- `pods/exec`, `pods/portforward`, `serviceaccounts/token`, `clusterrolebindings`는 운영 보안에서 우선 감시 대상임.
- EKS는 Kubernetes audit log와 EKS authenticator log를 함께 봐야 IAM 주체와 Kubernetes username/group 매핑을 추적하기 쉬움.
- 감사 로그 저장소 접근 권한을 cluster-admin과 분리함. 공격자가 cluster-admin을 얻은 뒤 audit log를 삭제하거나 변조하는 경로를 줄여야 함.
- control plane 노드 로컬 파일에만 저장하면 노드 장애·침해 시 증거가 사라짐. 운영 환경은 중앙 로그 저장소로 전달함.
- 공식 문서:
  - [Kubernetes Auditing](https://kubernetes.io/docs/tasks/debug/debug-cluster/audit/)
  - [Kubernetes audit.k8s.io/v1 API reference](https://kubernetes.io/docs/reference/config-api/apiserver-audit.v1/)
  - [Amazon EKS control plane logs](https://docs.aws.amazon.com/eks/latest/userguide/control-plane-logs.html)
