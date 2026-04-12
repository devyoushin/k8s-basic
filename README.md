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
| [rbac-advanced.md](deep-dive/rbac-advanced.md) | RBAC 심화 — Aggregated ClusterRole, OIDC 연동, Audit Policy |
| [node-affinity-taint.md](deep-dive/node-affinity-taint.md) | Node Affinity & Taint 심화 — 내부 매칭 규칙, GPU 노드 예약, HA 패턴 |
| [etcd-raft.md](deep-dive/etcd-raft.md) | etcd 심화 — 내부 Key-Value 구조, Raft 알고리즘, 운영 및 복구 |
| [apiserver-authn-etcd.md](deep-dive/apiserver-authn-etcd.md) | API Server 인증 심화 — X.509 / SA Token / OIDC 흐름, etcd 저장 구조, Watch 메커니즘 |
| [containerd-kernel.md](deep-dive/containerd-kernel.md) | containerd & Linux 커널 심화 — Namespace, Cgroup, OverlayFS, Seccomp, Capabilities |
| [pod-lifecycle.md](deep-dive/pod-lifecycle.md) | 파드 생명주기 심화 — 생성 시퀀스, kubelet Reconcile Loop, Probe 동작, Graceful Shutdown |
| [controller-informer.md](deep-dive/controller-informer.md) | 컨트롤러 패턴 심화 — Informer, SharedIndexInformer, WorkQueue, Reconcile, Operator 구현 |
| [network-packet-flow.md](deep-dive/network-packet-flow.md) | 네트워크 패킷 흐름 심화 — veth/브릿지, VXLAN, BGP, iptables DNAT, eBPF/Cilium |
| [storage-csi.md](deep-dive/storage-csi.md) | CSI 스토리지 심화 — 드라이버 구조, 동적 프로비저닝 흐름, 볼륨 확장, 스냅샷 |
| [tls-pki.md](deep-dive/tls-pki.md) | TLS & PKI 심화 — 클러스터 인증서 계층, 갱신, cert-manager, mTLS |
| [garbage-collection-finalizer.md](deep-dive/garbage-collection-finalizer.md) | 가비지 컬렉션 & Finalizer — OwnerReference, GC 알고리즘, Finalizer 패턴, 이미지 GC |
| [scheduler-internals.md](deep-dive/scheduler-internals.md) | 스케줄러 내부 심화 — 플러그인 파이프라인, Filter/Score 알고리즘, 선점(Preemption) |
| [oom-eviction.md](deep-dive/oom-eviction.md) | OOM & Eviction 심화 — QoS 클래스, OOM Score, kubelet Eviction Manager, 노드 압박 |
| [dns-service-discovery.md](deep-dive/dns-service-discovery.md) | DNS & 서비스 디스커버리 심화 — Headless Service, ndots 문제, CoreDNS 설정, ExternalName |
| [hpa-keda-autoscaling.md](deep-dive/hpa-keda-autoscaling.md) | 오토스케일링 심화 — HPA 알고리즘, VPA, KEDA 이벤트 기반, Cluster Autoscaler |
| [multitenancy-isolation.md](deep-dive/multitenancy-isolation.md) | 멀티테넌시 & 격리 — ResourceQuota, NetworkPolicy, 노드 격리, vCluster, HNC |
| [cluster-upgrade.md](deep-dive/cluster-upgrade.md) | 클러스터 업그레이드 — 버전 스큐 정책, kubeadm 절차, 노드 교체 방식, 롤백 |
| [backup-disaster-recovery.md](deep-dive/backup-disaster-recovery.md) | 백업 & 재해 복구 — etcd 스냅샷, Velero, DR 시나리오별 대응 |
| [gitops-argocd.md](deep-dive/gitops-argocd.md) | GitOps & ArgoCD — Sync 메커니즘, App of Apps, ApplicationSet, 롤백 |
| [supply-chain-security.md](deep-dive/supply-chain-security.md) | 공급망 보안 — cosign 이미지 서명, SBOM, SLSA, Kyverno 정책 검증 |
| [runtime-security-falco.md](deep-dive/runtime-security-falco.md) | 런타임 보안 & Falco — eBPF 이벤트 탐지, 룰 작성, Falcosidekick, 자동 대응 |
| [gateway-api.md](deep-dive/gateway-api.md) | Gateway API — GatewayClass/Gateway/HTTPRoute 역할 분리, 카나리, GRPCRoute |
| [node-local-dns.md](deep-dive/node-local-dns.md) | NodeLocal DNSCache — conntrack 경쟁 해결, 노드별 캐시, 성능 측정 |
| [crd-webhook-development.md](deep-dive/crd-webhook-development.md) | CRD & Webhook 개발 — 스키마 설계, Validating/Mutating Webhook, kubebuilder |
| [statefulset-patterns.md](deep-dive/statefulset-patterns.md) | StatefulSet 운영 패턴 — volumeClaimTemplate, Primary/Replica 구성, PVC 관리 |
| [ebpf-observability.md](deep-dive/ebpf-observability.md) | eBPF 관찰 가능성 — Hubble 서비스 맵, Pixie 코드리스 APM, Tetragon 보안 추적 |
| [deployment-strategy.md](deep-dive/deployment-strategy.md) | 배포 전략 심화 — Rolling/Recreate/Blue-Green/Canary, NLB Unhealthy 원인 분석, Resource Request 증가 시 대응 |
| [graceful-shutdown.md](deep-dive/graceful-shutdown.md) | Graceful Shutdown & 무중단 배포 — preStop Hook, terminationGracePeriod, Endpoint 전파 지연, Connection Draining |
| [eks-networking.md](deep-dive/eks-networking.md) | EKS 네트워킹 심화 — VPC CNI/ENI, NLB/ALB 타깃 모드, 보안그룹 파드 연결, IP 소진 대응 |
