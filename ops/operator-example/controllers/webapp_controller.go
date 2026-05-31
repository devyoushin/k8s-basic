package controllers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1alpha1 "github.com/devyoushin/webapp-operator/api/v1alpha1"
)

const webAppFinalizer = "apps.example.com/finalizer"

// WebAppReconciler: WebApp 리소스의 Reconcile 루프를 담당하는 컨트롤러
type WebAppReconciler struct {
	client.Client        // K8s API 서버와 통신 (Get/List/Create/Update/Delete)
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps.example.com,resources=webapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.example.com,resources=webapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile: K8s의 핵심 패턴 — "원하는 상태"와 "현재 상태"를 비교해서 일치시킵니다.
//
// 호출 시점:
// - WebApp 리소스가 생성/수정/삭제될 때
// - 컨트롤러가 관리하는 Deployment/Service가 변경될 때
// - 주기적인 재동기화 (resync) 시
func (r *WebAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconcile 시작", "webapp", req.NamespacedName)

	// ─────────────────────────────────────────────
	// Step 1. WebApp 리소스 가져오기
	// ─────────────────────────────────────────────
	webapp := &appsv1alpha1.WebApp{}
	if err := r.Get(ctx, req.NamespacedName, webapp); err != nil {
		if errors.IsNotFound(err) {
			// 리소스가 삭제된 경우 — 이미 GC가 처리하므로 무시
			logger.Info("WebApp 리소스를 찾을 수 없음 (이미 삭제됨)")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("WebApp 조회 실패: %w", err)
	}

	// ─────────────────────────────────────────────
	// Step 2. Finalizer 처리 (삭제 시 정리 작업)
	// ─────────────────────────────────────────────
	if webapp.DeletionTimestamp != nil {
		// 삭제 요청이 온 경우 — Finalizer 정리 후 실제 삭제 허용
		if controllerutil.ContainsFinalizer(webapp, webAppFinalizer) {
			logger.Info("Finalizer 정리 중", "webapp", webapp.Name)
			// 여기서 외부 리소스(예: AWS 리소스) 정리 로직 추가 가능

			controllerutil.RemoveFinalizer(webapp, webAppFinalizer)
			if err := r.Update(ctx, webapp); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Finalizer 등록 (아직 없으면 추가)
	if !controllerutil.ContainsFinalizer(webapp, webAppFinalizer) {
		controllerutil.AddFinalizer(webapp, webAppFinalizer)
		if err := r.Update(ctx, webapp); err != nil {
			return ctrl.Result{}, err
		}
	}

	// ─────────────────────────────────────────────
	// Step 3. Deployment 조정 (없으면 생성, 있으면 업데이트)
	// ─────────────────────────────────────────────
	deployment, err := r.reconcileDeployment(ctx, webapp)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("Deployment 조정 실패: %w", err)
	}

	// ─────────────────────────────────────────────
	// Step 4. Service 조정
	// ─────────────────────────────────────────────
	if err := r.reconcileService(ctx, webapp); err != nil {
		return ctrl.Result{}, fmt.Errorf("Service 조정 실패: %w", err)
	}

	// ─────────────────────────────────────────────
	// Step 5. Status 업데이트 (현재 상태를 WebApp.Status에 기록)
	// ─────────────────────────────────────────────
	if err := r.updateStatus(ctx, webapp, deployment); err != nil {
		return ctrl.Result{}, fmt.Errorf("Status 업데이트 실패: %w", err)
	}

	logger.Info("Reconcile 완료",
		"webapp", webapp.Name,
		"readyReplicas", deployment.Status.ReadyReplicas,
	)
	return ctrl.Result{}, nil
}

// reconcileDeployment: WebApp에 대응하는 Deployment를 생성하거나 업데이트합니다.
func (r *WebAppReconciler) reconcileDeployment(ctx context.Context, webapp *appsv1alpha1.WebApp) (*appsv1.Deployment, error) {
	logger := log.FromContext(ctx)

	// 원하는 Deployment 상태 정의
	desired := r.buildDeployment(webapp)

	// OwnerReference 설정:
	// WebApp이 삭제되면 Deployment도 자동으로 GC(가비지 컬렉션)됨
	if err := controllerutil.SetControllerReference(webapp, desired, r.Scheme); err != nil {
		return nil, err
	}

	// 현재 존재하는 Deployment 조회
	existing := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      desired.Name,
		Namespace: desired.Namespace,
	}, existing)

	if errors.IsNotFound(err) {
		// 없으면 생성
		logger.Info("Deployment 생성", "name", desired.Name)
		if err := r.Create(ctx, desired); err != nil {
			return nil, err
		}
		return desired, nil
	}
	if err != nil {
		return nil, err
	}

	// 있으면 스펙이 달라진 경우만 업데이트
	if deploymentNeedsUpdate(existing, webapp) {
		existing.Spec.Replicas = &webapp.Spec.Replicas
		existing.Spec.Template.Spec.Containers[0].Image = webapp.Spec.Image
		logger.Info("Deployment 업데이트", "name", existing.Name)
		if err := r.Update(ctx, existing); err != nil {
			return nil, err
		}
	}
	return existing, nil
}

// reconcileService: WebApp에 대응하는 Service를 생성하거나 업데이트합니다.
func (r *WebAppReconciler) reconcileService(ctx context.Context, webapp *appsv1alpha1.WebApp) error {
	logger := log.FromContext(ctx)

	desired := r.buildService(webapp)
	if err := controllerutil.SetControllerReference(webapp, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      desired.Name,
		Namespace: desired.Namespace,
	}, existing)

	if errors.IsNotFound(err) {
		logger.Info("Service 생성", "name", desired.Name)
		return r.Create(ctx, desired)
	}
	return err
}

// updateStatus: WebApp.Status를 현재 Deployment 상태로 업데이트합니다.
func (r *WebAppReconciler) updateStatus(ctx context.Context, webapp *appsv1alpha1.WebApp, deployment *appsv1.Deployment) error {
	// 최신 Deployment 상태 다시 조회
	current := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      deployment.Name,
		Namespace: deployment.Namespace,
	}, current); err != nil {
		return err
	}

	// ReadyReplicas 동기화
	webapp.Status.ReadyReplicas = current.Status.ReadyReplicas

	// Condition 설정: Available
	availableCondition := metav1.Condition{
		Type:               "Available",
		ObservedGeneration: webapp.Generation,
		LastTransitionTime: metav1.Now(),
	}
	if current.Status.ReadyReplicas == webapp.Spec.Replicas {
		availableCondition.Status = metav1.ConditionTrue
		availableCondition.Reason = "DeploymentAvailable"
		availableCondition.Message = fmt.Sprintf("%d/%d 파드 준비 완료", current.Status.ReadyReplicas, webapp.Spec.Replicas)
	} else {
		availableCondition.Status = metav1.ConditionFalse
		availableCondition.Reason = "DeploymentUnavailable"
		availableCondition.Message = fmt.Sprintf("%d/%d 파드만 준비됨", current.Status.ReadyReplicas, webapp.Spec.Replicas)
	}

	// 기존 Condition 교체 또는 추가
	setCondition(&webapp.Status.Conditions, availableCondition)

	return r.Status().Update(ctx, webapp)
}

// ─────────────────────────────────────────────
// 헬퍼 함수들
// ─────────────────────────────────────────────

// buildDeployment: WebApp 스펙으로 Deployment 오브젝트를 생성합니다.
func (r *WebAppReconciler) buildDeployment(webapp *appsv1alpha1.WebApp) *appsv1.Deployment {
	replicas := webapp.Spec.Replicas
	labels := map[string]string{
		"app":                     webapp.Name,
		"app.kubernetes.io/name":  webapp.Name,
		"managed-by":              "webapp-operator",
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webapp.Name,
			Namespace: webapp.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: intOrStringPtr(intstr.FromInt(0)),
					MaxSurge:       intOrStringPtr(intstr.FromInt(1)),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					// 파드 종료 시 30초 유예 (graceful shutdown)
					TerminationGracePeriodSeconds: int64Ptr(30),
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: webapp.Spec.Image,
							Ports: []corev1.ContainerPort{
								{ContainerPort: webapp.Spec.Port, Protocol: corev1.ProtocolTCP},
							},
							// NLB/ALB 연동 시 preStop sleep으로 Endpoint 전파 지연 보완
							Lifecycle: &corev1.Lifecycle{
								PreStop: &corev1.LifecycleHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "sleep 10"},
									},
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt(int(webapp.Spec.Port)),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								FailureThreshold:    3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt(int(webapp.Spec.Port)),
									},
								},
								InitialDelaySeconds: 15,
								PeriodSeconds:       10,
								FailureThreshold:    3,
							},
						},
					},
				},
			},
		},
	}
}

// buildService: WebApp 스펙으로 ClusterIP Service를 생성합니다.
func (r *WebAppReconciler) buildService(webapp *appsv1alpha1.WebApp) *corev1.Service {
	labels := map[string]string{
		"app":        webapp.Name,
		"managed-by": "webapp-operator",
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webapp.Name,
			Namespace: webapp.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt(int(webapp.Spec.Port)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// deploymentNeedsUpdate: Deployment 스펙이 WebApp과 다른지 확인합니다.
func deploymentNeedsUpdate(deployment *appsv1.Deployment, webapp *appsv1alpha1.WebApp) bool {
	if deployment.Spec.Replicas == nil {
		return true
	}
	if *deployment.Spec.Replicas != webapp.Spec.Replicas {
		return true
	}
	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		return true
	}
	if deployment.Spec.Template.Spec.Containers[0].Image != webapp.Spec.Image {
		return true
	}
	return false
}

// setCondition: Conditions 슬라이스에서 같은 Type의 항목을 교체하거나 추가합니다.
func setCondition(conditions *[]metav1.Condition, newCondition metav1.Condition) {
	for i, c := range *conditions {
		if c.Type == newCondition.Type {
			(*conditions)[i] = newCondition
			return
		}
	}
	*conditions = append(*conditions, newCondition)
}

func int64Ptr(i int64) *int64 { return &i }
func intOrStringPtr(v intstr.IntOrString) *intstr.IntOrString { return &v }

// SetupWithManager: 이 컨트롤러가 어떤 리소스의 변경을 감시할지 등록합니다.
func (r *WebAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// 주요 감시 대상: WebApp CRD
		For(&appsv1alpha1.WebApp{}).
		// 소유한 리소스 감시: Deployment/Service 변경 시에도 Reconcile 트리거
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
