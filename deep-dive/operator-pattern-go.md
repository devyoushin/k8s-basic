## 1. 개요 및 비유

**Kubernetes Operator**는 특정 애플리케이션의 운영 노하우(설치, 업그레이드, 장애 복구)를 코드로 표현한 컨트롤러 + CRD 조합입니다.

💡 **비유하자면 '자동 운영 매니저'와 같습니다.**
숙련된 시스템 운영자(Operator)가 하던 일 — "DB Primary가 죽으면 Replica를 Primary로 승격시켜라", "파드가 3개 미만이면 즉시 알림을 보내라" — 를 코드로 자동화합니다.

> 이 문서에 대응하는 실제 Go 코드: [`operator-example/`](../operator-example/)

---

## 2. 핵심 설명

### Operator = CRD + Controller

```
사용자가 선언:                 컨트롤러가 처리:
┌──────────────────┐          ┌────────────────────────────────┐
│ kind: WebApp     │  watch   │ Reconcile()                    │
│ spec:            │ ──────→  │   현재 상태 조회               │
│   image: nginx   │          │   원하는 상태와 비교           │
│   replicas: 3    │          │   Deployment/Service 생성/수정 │
└──────────────────┘          │   Status 업데이트              │
                              └────────────────────────────────┘
```

### Reconcile 루프 — 핵심 패턴

```go
func (r *Reconciler) Reconcile(ctx context.Context, req Request) (Result, error) {
    // 1. 현재 리소스 상태 가져오기 (Get)
    // 2. 삭제 중이면 Finalizer 정리 후 리턴
    // 3. 원하는 상태와 현재 상태 비교
    // 4. 차이가 있으면 Create/Update로 일치시키기
    // 5. Status 업데이트
}
```

**멱등성(Idempotency):** Reconcile은 언제 몇 번 호출되어도 같은 결과여야 합니다.
- 이미 원하는 상태이면 아무것도 하지 않음
- 실패해도 재시도하면 수렴

### controller-runtime 핵심 컴포넌트

| 컴포넌트 | 역할 |
|---|---|
| `Manager` | 컨트롤러, 캐시(Informer), 헬스체크, 메트릭 통합 관리 |
| `Client` | K8s API 서버와 통신 (Get/List/Create/Update/Delete) |
| `Cache` | API 서버 응답을 메모리에 캐싱 (Watch 기반) — Get/List가 캐시 조회 |
| `Reconciler` | 사용자가 구현하는 핵심 인터페이스 |
| `Builder` | 어떤 리소스 변경에 반응할지 선언 |

### OwnerReference — 자동 GC

```go
// WebApp이 소유하는 Deployment 설정
controllerutil.SetControllerReference(webapp, deployment, scheme)

// 결과: WebApp 삭제 시 K8s GC가 Deployment도 자동 삭제
// kubectl get deployment -l managed-by=webapp-operator 로 확인 가능
```

### Finalizer — 삭제 전 정리 작업

```go
// Finalizer가 있으면 실제 삭제가 지연됨 (DeletionTimestamp만 찍힘)
// 외부 리소스(AWS, DB 등) 정리 후 Finalizer 제거 → 그 후 실제 삭제
if webapp.DeletionTimestamp != nil {
    // 정리 작업 (예: AWS 리소스 삭제)
    controllerutil.RemoveFinalizer(webapp, finalizer)
    r.Update(ctx, webapp)  // 이후 K8s GC가 실제 삭제
}
```

---

## 3. 프로젝트 구조 및 실행 예시

### 디렉토리 구조
```
operator-example/
├── main.go                           # Manager 초기화 & 진입점
├── api/v1alpha1/
│   ├── groupversion_info.go          # API 그룹/버전 정의 (apps.example.com/v1alpha1)
│   └── webapp_types.go               # WebApp CRD 타입 (Spec, Status)
├── controllers/
│   └── webapp_controller.go          # Reconcile 루프 핵심 로직
├── config/
│   ├── crd/webapp.yaml               # CRD 매니페스트 (클러스터에 적용)
│   └── samples/webapp.yaml           # 샘플 CR
└── go.mod
```

### 로컬에서 실행하기

```bash
# 1. CRD 클러스터에 등록
kubectl apply -f config/crd/webapp.yaml

# 2. CRD 등록 확인
kubectl get crd webapps.apps.example.com

# 3. 컨트롤러 로컬 실행 (kubeconfig 사용)
go run main.go

# 4. 다른 터미널에서 WebApp 리소스 생성
kubectl apply -f config/samples/webapp.yaml

# 5. 생성된 리소스 확인
kubectl get webapp                        # CRD 목록 (추가 컬럼 포함)
kubectl get deployment,service my-webapp  # 컨트롤러가 생성한 리소스
kubectl describe webapp my-webapp         # Status.Conditions 확인
```

### 실행 예시 출력

```bash
$ kubectl get webapp
NAME        IMAGE        REPLICAS   READY   AGE
my-webapp   nginx:1.25   3          3       2m

$ kubectl describe webapp my-webapp
Name:         my-webapp
Namespace:    default
Spec:
  Image:     nginx:1.25
  Replicas:  3
  Port:      80
Status:
  Ready Replicas:  3
  Conditions:
    Type:    Available
    Status:  True
    Reason:  DeploymentAvailable
    Message: 3/3 파드 준비 완료
```

---

## 4. 트러블 슈팅

### Reconcile이 무한 루프에 빠짐

**원인:** Status 업데이트가 또 다른 Reconcile을 트리거함

```go
// 잘못된 예: r.Update()는 spec 변경 → Reconcile 재트리거
r.Update(ctx, webapp)  // ← spec/metadata 변경

// 올바른 예: Status만 업데이트할 때는 Status().Update() 사용
r.Status().Update(ctx, webapp)  // ← status 서브리소스만 변경 (Reconcile 재트리거 없음)
```

---

### "object has been modified; please apply your changes to the latest version"

**원인:** Get → Update 사이에 다른 주체가 리소스를 변경한 충돌 (resourceVersion 불일치)

```go
// 해결: Retry 패턴 적용
import "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
    // 매번 최신 상태를 다시 Get
    if err := r.Get(ctx, req.NamespacedName, webapp); err != nil {
        return err
    }
    webapp.Spec.Replicas = newReplicas
    return r.Update(ctx, webapp)
})
```

---

### 컨트롤러가 CRD 변경을 감지 못함

**원인:** `SetupWithManager`에서 감시 대상 리소스 누락

```go
// 소유한 리소스 변경도 Reconcile 트리거하려면 Owns() 추가
ctrl.NewControllerManagedBy(mgr).
    For(&appsv1alpha1.WebApp{}).
    Owns(&appsv1.Deployment{}).   // Deployment 변경 시 WebApp Reconcile 트리거
    Owns(&corev1.Service{}).      // Service 변경 시 WebApp Reconcile 트리거
    Complete(r)
```

---

### Operator 개발 심화 학습 경로

```
1단계 (이 예제): controller-runtime 직접 사용
  → CRD 타입 직접 작성, Reconcile 로직 이해

2단계: kubebuilder 스캐폴딩
  kubebuilder init --domain example.com --repo github.com/foo/bar
  kubebuilder create api --group apps --version v1alpha1 --kind WebApp
  → CRD 스키마, RBAC, Webhook, Makefile 자동 생성

3단계: Operator SDK (kubebuilder 기반)
  → Ansible/Helm operator 지원, OLM(Operator Lifecycle Manager) 통합

4단계: 실제 프로덕션 패턴
  → Leader Election (고가용성), Webhook (유효성 검사)
  → 멀티 버전 CRD 변환, 외부 시스템 연동 Finalizer
```
