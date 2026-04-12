package main

import (
	"flag"
	"os"

	// K8s 기본 API 타입 (Deployment, Service 등)
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	// controller-runtime: operator 개발 프레임워크
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	// 우리가 정의한 WebApp CRD 타입
	appsv1alpha1 "github.com/devyoushin/webapp-operator/api/v1alpha1"
	"github.com/devyoushin/webapp-operator/controllers"
)

var (
	// scheme: 어떤 K8s 타입을 다룰지 등록하는 레지스트리
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	// 기본 K8s 타입 등록 (Pod, Deployment, Service 등)
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(appsv1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))

	// 우리가 만든 WebApp CRD 타입 등록
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
}

func main() {
	// ─────────────────────────────────────────────
	// 플래그 파싱
	// ─────────────────────────────────────────────
	var (
		metricsAddr          string
		probeAddr            string
		enableLeaderElection bool
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "메트릭 서버 주소")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "헬스체크 프로브 주소")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"리더 선출 활성화 — 컨트롤러를 여러 개 띄울 때 1개만 활성화되도록 보장")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// ─────────────────────────────────────────────
	// Manager 생성
	// Manager: 컨트롤러 실행, 캐시(Informer), 헬스체크, 메트릭을 통합 관리
	// ─────────────────────────────────────────────
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		// 리더 선출: 여러 replica로 operator를 배포할 때 하나만 active로 동작
		LeaderElection:   enableLeaderElection,
		LeaderElectionID: "webapp-operator.apps.example.com",
	})
	if err != nil {
		setupLog.Error(err, "Manager 생성 실패")
		os.Exit(1)
	}

	// ─────────────────────────────────────────────
	// 컨트롤러 등록
	// ─────────────────────────────────────────────
	if err = (&controllers.WebAppReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "WebApp 컨트롤러 등록 실패")
		os.Exit(1)
	}

	// ─────────────────────────────────────────────
	// 헬스체크 엔드포인트 등록
	// kubectl get deployment -n operator-system 에서 Ready 상태 확인용
	// ─────────────────────────────────────────────
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "헬스체크 등록 실패")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "레디체크 등록 실패")
		os.Exit(1)
	}

	// ─────────────────────────────────────────────
	// Manager 시작 (블로킹 — SIGTERM까지 대기)
	// ─────────────────────────────────────────────
	setupLog.Info("WebApp Operator 시작")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager 실행 실패")
		os.Exit(1)
	}
}
