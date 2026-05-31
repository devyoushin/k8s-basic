## 1. 개요 및 비유

Kubernetes의 가비지 컬렉션(GC)은 부모 리소스가 삭제될 때 고아가 된 자식 리소스를 자동 정리합니다. Finalizer는 리소스 삭제 전 반드시 수행해야 할 작업을 보장합니다.

💡 **비유하자면 아파트 퇴거 정리와 같습니다.**
세입자(자식 리소스)는 집주인(부모 리소스) 퇴거 시 함께 나가야 합니다. Finalizer는 "보증금 반환 확인서"처럼 이를 받기 전까지 퇴거 처리(삭제)를 막습니다.

---

## 2. OwnerReference — 리소스 소유 관계

### 2.1 소유 관계 체인

```
Deployment
  └── ReplicaSet        (ownerReference → Deployment)
        └── Pod × 3    (ownerReference → ReplicaSet)

모든 파드 metadata에:
ownerReferences:
- apiVersion: apps/v1
  kind: ReplicaSet
  name: my-deploy-7d9f8b6c4
  uid: abc-123-...
  controller: true        ← 이 소유자가 제어권을 가짐
  blockOwnerDeletion: true ← 소유자 삭제 시 자식 먼저 삭제 대기
```

```bash
# 파드의 ownerReference 확인
kubectl get pod my-pod -o jsonpath='{.metadata.ownerReferences}' | jq .

# 특정 ReplicaSet이 소유한 파드 목록
kubectl get pods --field-selector=metadata.ownerReferences.name=my-deploy-7d9f8b6c4

# 또는 label selector로 (ownerReference와 label이 연동됨)
kubectl get pods -l app=my-app
```

### 2.2 GC 컨트롤러 동작 원리

```
GarbageCollector Controller 루프:
┌──────────────────────────────────────────────────────────┐
│ 1. 모든 리소스의 ownerReference를 DAG(유향 비순환 그래프)  │
│    으로 관리                                              │
│                                                          │
│ 2. 부모 삭제 감지 시:                                    │
│    - 자식들의 ownerReference에서 부모 UID 조회            │
│    - 해당 부모가 존재하지 않으면 → 고아(orphan) 감지      │
│                                                          │
│ 3. propagationPolicy에 따라:                             │
│    Foreground → 자식 먼저 삭제 후 부모 삭제              │
│    Background → 부모 먼저 삭제, 자식은 GC가 비동기 정리  │
│    Orphan     → 자식의 ownerReference만 제거 (자식 유지) │
└──────────────────────────────────────────────────────────┘
```

```bash
# Background 삭제 (기본값 — 부모 먼저 사라짐)
kubectl delete deployment my-deploy
# → Deployment 즉시 사라짐, ReplicaSet/Pod는 GC가 비동기 정리

# Foreground 삭제 (자식이 모두 삭제될 때까지 부모 남음)
kubectl delete deployment my-deploy \
  --cascade=foreground
# → Deployment에 foregroundDeletion finalizer 추가
# → 자식 Pod → ReplicaSet 삭제 완료 후 Deployment 사라짐

# Orphan 삭제 (자식 유지, 소유 관계만 끊음)
kubectl delete deployment my-deploy \
  --cascade=orphan
# → ReplicaSet/Pod는 남아있지만 ownerReference 제거됨
# → 이후 독립적으로 동작 (GC에 의해 자동 삭제되지 않음)
```

---

## 3. Finalizer 심층 동작

### 3.1 Finalizer가 있는 리소스의 삭제 흐름

```
kubectl delete pvc my-pvc
        │
        ▼
API Server:
  - PVC에 metadata.deletionTimestamp 추가
  - 하지만 metadata.finalizers에 항목이 있으면 실제 삭제 안 함
  - PVC 상태: Terminating

예: metadata.finalizers: ["kubernetes.io/pvc-protection"]
        │
        ▼
pvc-protection 컨트롤러 감지:
  - 이 PVC를 사용 중인 파드가 있는지 확인
  - 파드가 있으면: finalizer 유지 (삭제 차단 계속)
  - 파드가 없으면: finalizer 제거

        │ finalizer 제거됨
        ▼
API Server: finalizers 빈 배열 → 실제 삭제 수행
etcd에서 PVC 오브젝트 완전 제거
```

```bash
# Terminating 상태의 리소스 finalizer 확인
kubectl get pvc my-pvc -o jsonpath='{.metadata.finalizers}'
# ["kubernetes.io/pvc-protection"]

# 어떤 파드가 PVC를 사용 중인지 확인
kubectl get pods -o json | jq '.items[] | select(.spec.volumes[]?.persistentVolumeClaim.claimName == "my-pvc") | .metadata.name'

# PVC 사용 파드 삭제 후 → PVC 자동 삭제됨
```

### 3.2 주요 내장 Finalizer 목록

| Finalizer | 리소스 | 역할 |
|---|---|---|
| `kubernetes.io/pvc-protection` | PVC | 사용 중인 파드 있으면 삭제 차단 |
| `kubernetes.io/pv-protection` | PV | 바인딩된 PVC 있으면 삭제 차단 |
| `foregroundDeletion` | 모든 리소스 | Foreground 삭제 시 자동 추가 |
| `orphan` | 모든 리소스 | Orphan 삭제 시 자동 추가 |
| `service.kubernetes.io/load-balancer-cleanup` | Service | 로드밸런서 리소스 정리 후 삭제 |

### 3.3 커스텀 Finalizer 구현 패턴

```go
// 컨트롤러에서 Finalizer 관리 패턴
const myFinalizer = "mycompany.io/external-resource-cleanup"

func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &myv1.MyResource{}
    r.Get(ctx, req.NamespacedName, obj)

    // 삭제 요청 감지
    if !obj.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(obj, myFinalizer) {
            // 외부 리소스 정리 (S3 버킷, RDS 인스턴스 등)
            if err := r.deleteExternalResources(obj); err != nil {
                // 실패 시 재시도 (finalizer 유지)
                return ctrl.Result{}, err
            }
            // 정리 완료 → finalizer 제거
            controllerutil.RemoveFinalizer(obj, myFinalizer)
            r.Update(ctx, obj)
        }
        return ctrl.Result{}, nil
    }

    // 최초 생성 시 finalizer 등록
    if !controllerutil.ContainsFinalizer(obj, myFinalizer) {
        controllerutil.AddFinalizer(obj, myFinalizer)
        return ctrl.Result{}, r.Update(ctx, obj)
    }

    // 정상 Reconcile 로직 ...
}
```

---

## 4. 이미지 가비지 컬렉션

kubelet은 노드의 디스크 공간을 보호하기 위해 사용하지 않는 컨테이너 이미지를 자동 삭제합니다.

```
이미지 GC 트리거 조건 (kubelet 설정):
imageGCHighThresholdPercent: 85  ← 디스크 85% 이상 사용 시 GC 시작
imageGCLowThresholdPercent: 80   ← 80%까지 이미지 삭제

삭제 우선순위:
1. 현재 실행 중인 컨테이너에서 사용하지 않는 이미지
2. 마지막 사용 시간이 오래된 이미지부터 삭제
(실행 중인 컨테이너의 이미지는 절대 삭제 안 함)
```

```bash
# 노드의 이미지 GC 설정 확인
cat /var/lib/kubelet/config.yaml | grep -i imagegc

# 현재 노드의 이미지 목록과 크기
crictl images

# 수동으로 사용하지 않는 이미지 정리
crictl rmi --prune

# 이미지 GC 이벤트 확인
kubectl get events --field-selector reason=FreeDiskSpaceFailed
kubectl describe node worker-1 | grep -A5 "Events:"
```

---

## 5. 트러블슈팅

* **리소스가 Terminating에서 영원히 멈춤:**
  ```bash
  # finalizer 목록 확인
  kubectl get <resource> <name> -o jsonpath='{.metadata.finalizers}'

  # 담당 컨트롤러가 살아있는지 확인
  kubectl get pods -n kube-system | grep <controller-name>

  # 최후 수단: finalizer 강제 제거 (데이터 손실 가능성 있음)
  kubectl patch <resource> <name> \
    -p '{"metadata":{"finalizers":null}}' \
    --type=merge
  ```

* **Deployment 삭제했는데 파드가 남아있음:**
  ```bash
  # --cascade=orphan으로 삭제된 경우
  kubectl get pods -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.metadata.ownerReferences}{"\n"}{end}'
  # ownerReferences가 없는 파드 = 고아 파드

  # 고아 파드 정리
  kubectl delete pod <orphan-pod-name>
  ```

* **PVC가 Terminating인데 파드는 없음:**
  ```bash
  # 종료 중인 파드(Terminating) 확인 — 완전히 삭제 전까지 PVC 유지
  kubectl get pods --field-selector=status.phase=Terminating

  # 파드 강제 삭제
  kubectl delete pod <pod-name> --grace-period=0 --force
  ```
