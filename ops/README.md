# Kubernetes Ops

Kubernetes 실습 코드와 실행 자산은 이 디렉터리에서 관리합니다.

| 폴더 | 내용 |
|------|------|
| `scripts/` | kubectl 기반 클러스터/워크로드 진단 스크립트 |
| `manifests/` | Pod, Deployment, Service, RBAC, NetworkPolicy 예제 |
| `labs/` | DNS, scheduling, rollout, resource, storage 실습 |
| `runbooks/` | Kubernetes 장애 상황별 대응 절차 |
| `checklists/` | 클러스터/배포/보안 점검 체크리스트 |
| `configs/` | kubeadm, Kubespray, kind, kubectl, plugin 설정 예시 |
| `outputs/` | 실습 결과와 kubectl 출력 샘플 |
| `operator-example/` | controller-runtime 기반 Go Operator 예제 |

문서와 설명은 `../docs/README.md`를 참고합니다.

## 빠른 실행

```bash
bash ops/scripts/cluster-summary.sh
bash ops/scripts/node-summary.sh
bash ops/scripts/workload-summary.sh
bash ops/scripts/events-recent.sh
bash ops/scripts/efs-pod-map.sh
```

스크립트는 기본적으로 조회 전용입니다. 실제 리소스를 생성하는 실습은 `ops/manifests/`와 `ops/labs/`에서 별도로 관리합니다.
