# k8s-basic

Kubernetes 핵심 개념을 한국어로 쉽게 정리한 학습 레포지토리입니다.

```
k8s-basic/
├── components/   # 클러스터 구성 컴포넌트
├── objects/      # 쿠버네티스 오브젝트(리소스)
├── network/      # 네트워킹 심화
├── security/     # 보안 심화
└── deep-dive/    # 특정 주제 심층 분석
```

---

## components/ — 클러스터 컴포넌트

| 파일 | 설명 |
|---|---|
| [control-plane.md](components/control-plane.md) | 컨트롤 플레인 전체 개요 |
| [apiserver.md](components/apiserver.md) | API Server — 클러스터의 관문 |
| [etcd.md](components/etcd.md) | etcd — 클러스터 상태 저장소 |
| [scheduler.md](components/scheduler.md) | Scheduler — 파드 배치 결정자 |
| [controller-manager.md](components/controller-manager.md) | Controller Manager — 상태 감시 루프 |
| [kubelet.md](components/kubelet.md) | Kubelet — 노드의 현장 소장 |
| [kube-proxy.md](components/kube-proxy.md) | Kube-Proxy — 노드 네트워크 규칙 관리 |

---

## objects/ — 워크로드

| 파일 | 설명 |
|---|---|
| [pod.md](objects/pod.md) | Pod — 가장 작은 배포 단위 |
| [deployment.md](objects/deployment.md) | Deployment — 무중단 롤링 업데이트 |
| [statefulset.md](objects/statefulset.md) | StatefulSet — 상태 있는 애플리케이션 |
| [daemonset.md](objects/daemonset.md) | DaemonSet — 모든 노드에 1개씩 |
| [job.md](objects/job.md) | Job — 일회성 작업 실행 |
| [cronjob.md](objects/cronjob.md) | CronJob — 주기적 배치 작업 |

## objects/ — 서비스 & 네트워킹

| 파일 | 설명 |
|---|---|
| [service.md](objects/service.md) | Service — ClusterIP / NodePort / LoadBalancer |
| [ingress.md](objects/ingress.md) | Ingress — HTTP/HTTPS 라우팅 |

## objects/ — 스토리지

| 파일 | 설명 |
|---|---|
| [pv-pvc.md](objects/pv-pvc.md) | PersistentVolume / PersistentVolumeClaim |

## objects/ — 설정 & 보안

| 파일 | 설명 |
|---|---|
| [configmap.md](objects/configmap.md) | ConfigMap — 설정 분리 |
| [secret.md](objects/secret.md) | Secret — 민감 데이터 관리 |
| [namespace.md](objects/namespace.md) | Namespace — 클러스터 논리적 분리 |
| [rbac.md](objects/rbac.md) | RBAC — 역할 기반 접근 제어 |

## objects/ — 스케일링 & 안정성

| 파일 | 설명 |
|---|---|
| [hpa.md](objects/hpa.md) | HPA — 수평적 파드 오토스케일링 |
| [pdb.md](objects/pdb.md) | PodDisruptionBudget — 자발적 중단 시 최소 가용성 보장 |

---

## network/ — 네트워킹 심화

| 파일 | 설명 |
|---|---|
| [cni-service-proxy.md](network/cni-service-proxy.md) | CNI & Service Proxy 동작 원리 |
| [ipvs.md](network/ipvs.md) | IPVS 모드 심화 |
| [network-policy.md](network/network-policy.md) | NetworkPolicy — 파드 간 트래픽 방화벽 |
| [coredns.md](network/coredns.md) | CoreDNS — 클러스터 DNS & ndots 이슈 |
| [service-mesh.md](network/service-mesh.md) | Service Mesh (Istio) — mTLS, 카나리, 서킷 브레이커 |

---

## security/ — 보안 심화

| 파일 | 설명 |
|---|---|
| [security-context.md](security/security-context.md) | SecurityContext — runAsNonRoot, Capabilities, Seccomp |
| [image-security.md](security/image-security.md) | 컨테이너 이미지 보안 — 취약점 스캔, 서명, distroless |
| [pod-security-standards.md](security/pod-security-standards.md) | Pod Security Standards — privileged / baseline / restricted |
| [secrets-management.md](security/secrets-management.md) | 시크릿 관리 심화 — ESO, Vault, etcd 암호화 |

---

## deep-dive/ — 심층 분석

| 파일 | 설명 |
|---|---|
| [apiserver-process.md](deep-dive/apiserver-process.md) | API Server 처리 과정 심화 |
| [container-runtime.md](deep-dive/container-runtime.md) | 컨테이너 런타임 — containerd, CRI, OCI, crictl |
| [scheduling-advanced.md](deep-dive/scheduling-advanced.md) | 고급 스케줄링 — Taint/Toleration, Affinity, PriorityClass |
| [resource-management.md](deep-dive/resource-management.md) | 리소스 관리 — QoS 클래스, VPA, Eviction |
| [observability.md](deep-dive/observability.md) | 관찰 가능성 — 로그(Fluent Bit), 메트릭(Prometheus), 트레이싱 |
| [admission-control.md](deep-dive/admission-control.md) | Admission Control — Kyverno, OPA Gatekeeper, Webhook |
| [kubectl-api-resources.md](deep-dive/kubectl-api-resources.md) | kubectl api-resources 전체 목록 |
