# k8s-basic

Kubernetes 핵심 개념과 운영 심화 주제를 한국어로 정리한 학습 레포지토리입니다.

## 어디서 시작할까

- 문서 지도: `docs/README.md`
- 설치/업그레이드: `docs/install/README.md`
- 실습 코드: `ops/README.md`
- AI 작업 지침: `CLAUDE.md`, `AGENTS.md -> CLAUDE.md`

## 구조

| 경로 | 내용 |
|------|------|
| `docs/` | 설치, 컴포넌트, 오브젝트, 네트워크, 보안, 심화 문서, 에이전트, 규칙, 템플릿 |
| `ops/` | 진단 스크립트, manifest 예제, lab, runbook, 체크리스트, Operator 실습 코드 |
| `.claude/` | Claude Code 커맨드와 설정 |
| `CLAUDE.md` | Claude/Codex 공통 작업 지침 원본 |
| `AGENTS.md -> CLAUDE.md` | Codex/agent 작업 지침 링크 |

## 학습 흐름

1. `docs/install/`에서 kubeadm, Kubespray, external etcd, upgrade 흐름 확인
2. `docs/components/`에서 Control Plane과 Node 구성 요소 이해
3. `docs/objects/`에서 Pod, Deployment, Service 등 핵심 리소스 학습
4. `docs/network/`, `docs/security/`로 운영 관점 확장
5. `docs/deep-dive/`에서 스케줄러, etcd, CNI, TLS, Operator 등 심화 학습
6. `ops/scripts/`, `ops/configs/`, `ops/runbooks/`로 운영 실습 진행
7. `ops/operator-example/`에서 Operator 구현 예제 확인
