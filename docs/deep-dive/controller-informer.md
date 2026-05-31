## 1. 개요 및 비유

Kubernetes의 모든 컨트롤러(Deployment, ReplicaSet, Node 등)는 동일한 **Informer + WorkQueue + Reconcile** 패턴으로 구현됩니다.

💡 **비유하자면 주문 처리 시스템과 같습니다.**
Informer(주문 접수 창구)가 변경 사항을 감지 → WorkQueue(대기열)에 쌓음 → Worker(처리자)가 꺼내서 Reconcile(실제 작업) 수행. 처리 실패 시 자동 재시도.

---

## 2. Informer 내부 구조

### 2.1 전체 아키텍처

```
API Server
    │
    │  List + Watch (HTTP/2 스트리밍)
    ▼
┌─────────────────────────────────────────────────────┐
│  Reflector                                          │
│  - 초기: List로 전체 오브젝트 로드                   │
│  - 이후: Watch로 변경 이벤트만 수신                  │
│  - 연결 끊기면 resourceVersion 기반으로 재연결        │
└──────────────┬──────────────────────────────────────┘
               │ ADDED / MODIFIED / DELETED 이벤트
               ▼
┌─────────────────────────────────────────────────────┐
│  DeltaFIFO Queue (이벤트 버퍼)                      │
│  - 같은 오브젝트의 이벤트를 합산 (이벤트 스톰 방지)  │
│  - 순서 보장                                         │
└──────────────┬──────────────────────────────────────┘
               │
        ┌──────┴──────┐
        ▼             ▼
┌──────────────┐  ┌────────────────────────────────────┐
│  Indexer     │  │  EventHandler (콜백 등록)           │
│  (로컬 캐시) │  │  OnAdd / OnUpdate / OnDelete        │
│  - 메모리    │  │  → WorkQueue에 키(namespace/name)   │
│  - Thread    │  │    추가                             │
│    Safe      │  └────────────────┬───────────────────┘
└──────────────┘                   │
                                   ▼
                         ┌────────────────────┐
                         │  RateLimiting      │
                         │  WorkQueue         │
                         │  - 중복 키 자동 제거│
                         │  - 재시도 backoff  │
                         └────────┬───────────┘
                                  │
                                  ▼
                         Worker Goroutine들
                         Reconcile(key) 호출
```

### 2.2 왜 API Server를 직접 폴링하지 않는가

```
나쁜 패턴 (폴링):
controller → GET /api/v1/pods (매 5초마다)
→ API Server 부하 증가
→ 모든 파드 데이터 전송 (대역폭 낭비)
→ 변경 감지 최대 5초 지연

좋은 패턴 (Informer):
controller → List + Watch
→ 초기 1회 List로 캐시 구성
→ 이후 변경분만 Watch로 수신 (수 밀리초 이내)
→ 로컬 캐시(Indexer)에서 조회 → API Server 부하 0
```

### 2.3 SharedInformer — 같은 리소스를 여러 컨트롤러가 공유

```
비효율적: 각 컨트롤러가 별도 Watch 연결
  Deployment 컨트롤러 → Watch Pods
  ReplicaSet 컨트롤러 → Watch Pods   (중복!)
  DaemonSet 컨트롤러 → Watch Pods   (중복!)

SharedIndexInformer:
  단일 Watch 연결
  ├── Deployment 컨트롤러 핸들러 등록
  ├── ReplicaSet 컨트롤러 핸들러 등록
  └── DaemonSet 컨트롤러 핸들러 등록
  → 하나의 이벤트를 모든 핸들러에 브로드캐스트
```

---

## 3. Reconcile 패턴 심층 분석

### 3.1 선언적 제어의 핵심 — 멱등성(Idempotency)

```
Reconcile 함수의 기본 구조:
func Reconcile(key string) error {
    // 1. 캐시에서 현재 상태 조회 (API Server 아님!)
    desired, err := lister.Get(key)

    // 2. 실제 상태 조회 (필요시 API 호출)
    actual := getActualState()

    // 3. 차이 계산 및 조정
    if desired != actual {
        makeChange() // API 호출로 실제 변경
    }
    return nil
}

핵심 특성:
- 같은 key로 100번 호출해도 결과 동일 (멱등성)
- 변경 사항이 없으면 아무것도 안 함
- 에러 발생 시 WorkQueue가 자동 재시도
```

### 3.2 Deployment 컨트롤러 Reconcile 실제 흐름

```
Deployment 변경 감지 (예: replicas 3→5)
        │
        ▼
Reconcile("default/my-deploy") 호출
        │
        ▼
현재 ReplicaSet 조회
   - desired: replicas=5
   - actual: replicas=3
        │
        ▼
ReplicaSet 업데이트 (replicas=5)
        │
        ▼ Watch 이벤트
ReplicaSet 컨트롤러 Reconcile 호출
        │
        ▼
현재 Pod 수 조회 (캐시에서)
   - desired: 5
   - actual: 3
        │
        ▼
파드 2개 생성 요청 → API Server
        │
        ▼
Scheduler → kubelet → 컨테이너 실행
        │
        ▼
파드 Running → Endpoints 업데이트
```

---

## 4. 커스텀 컨트롤러(Operator) 구현 패턴

### 4.1 CRD + Controller = Operator

```yaml
# CRD 정의 예시 (간략)
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: databases.mycompany.io
spec:
  group: mycompany.io
  names:
    kind: Database
    plural: databases
  scope: Namespaced
  versions:
  - name: v1
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              engine:
                type: string    # postgres, mysql
              replicas:
                type: integer
          status:
            type: object
            properties:
              phase:
                type: string   # Provisioning, Running, Failed
```

### 4.2 controller-runtime 기반 컨트롤러 구조 (Go)

```go
// Reconciler 인터페이스 구현
type DatabaseReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. CRD 인스턴스 조회
    db := &myv1.Database{}
    if err := r.Get(ctx, req.NamespacedName, db); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. StatefulSet 존재 여부 확인
    sts := &appsv1.StatefulSet{}
    err := r.Get(ctx, req.NamespacedName, sts)

    if errors.IsNotFound(err) {
        // 3. 없으면 생성
        newSts := r.buildStatefulSet(db)
        return ctrl.Result{}, r.Create(ctx, newSts)
    }

    // 4. 있으면 spec 비교 후 업데이트
    if sts.Spec.Replicas != db.Spec.Replicas {
        sts.Spec.Replicas = db.Spec.Replicas
        return ctrl.Result{}, r.Update(ctx, sts)
    }

    // 5. status 업데이트
    db.Status.Phase = "Running"
    return ctrl.Result{}, r.Status().Update(ctx, db)
}
```

### 4.3 Finalizer 패턴 — 삭제 전 정리 작업

```go
const myFinalizer = "mycompany.io/cleanup"

func (r *DatabaseReconciler) Reconcile(...) {
    // 삭제 요청 감지
    if !db.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(db, myFinalizer) {
            // 외부 리소스 정리 (예: 클라우드 DB 인스턴스 삭제)
            r.cleanupExternalResources(db)

            // finalizer 제거 → etcd에서 실제 삭제 진행
            controllerutil.RemoveFinalizer(db, myFinalizer)
            r.Update(ctx, db)
        }
        return ctrl.Result{}, nil
    }

    // 최초 생성 시 finalizer 추가
    if !controllerutil.ContainsFinalizer(db, myFinalizer) {
        controllerutil.AddFinalizer(db, myFinalizer)
        r.Update(ctx, db)
    }
    // ... 나머지 Reconcile 로직
}
```

---

## 5. 트러블슈팅

* **컨트롤러가 이벤트를 처리 못하고 쌓임:**
  ```bash
  # WorkQueue 길이 확인 (Prometheus 메트릭)
  # workqueue_depth{name="<controller-name>"}
  kubectl get --raw /metrics | grep workqueue_depth

  # 처리 속도 확인
  # workqueue_queue_duration_seconds (대기 시간)
  # workqueue_work_duration_seconds  (처리 시간)
  ```

* **Informer 캐시와 실제 상태 불일치 (캐시 지연):**
  ```bash
  # 컨트롤러가 stale 캐시를 읽는 경우
  # → API Server 직접 조회로 강제 (캐시 우회)
  # client.Get() 대신 APIReader 사용하거나
  # List 시 ResourceVersion: "0" 명시
  ```

* **CRD 설치 후 컨트롤러가 오브젝트를 못 찾음:**
  ```bash
  # CRD 등록 상태 확인
  kubectl get crd databases.mycompany.io
  kubectl get crd databases.mycompany.io -o jsonpath='{.status.conditions}'

  # Established 조건이 True인지 확인
  # 컨트롤러 재시작 (CRD 등록 전에 시작됐을 수 있음)
  kubectl rollout restart deployment my-operator
  ```
