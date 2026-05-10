# k8s-basic 프로젝트 가이드

## 프로젝트 목적
Kubernetes의 모든 핵심 컴포넌트, 오브젝트, 네트워킹 개념을 한국어로 쉽게 정리한 학습 레포지토리입니다.

---

## 디렉토리 구조

```
k8s-basic/
├── CLAUDE.md                  # 이 파일 (자동 로드)
├── .claude/
│   ├── settings.json          # 권한 설정 + PostToolUse 훅
│   └── commands/              # 커스텀 슬래시 명령어
│       ├── new-doc.md         # /new-doc — 새 문서 생성
│       ├── new-runbook.md     # /new-runbook — 새 런북 생성
│       ├── review-doc.md      # /review-doc — 문서 품질 검토
│       ├── add-troubleshooting.md  # /add-troubleshooting — 트러블슈팅 추가
│       └── search-kb.md       # /search-kb — 지식베이스 검색
├── agents/                    # 전문 에이전트 정의
│   ├── doc-writer.md          # K8s 문서 작성 전문가
│   ├── troubleshooter.md      # 장애 진단 전문가
│   ├── yaml-reviewer.md       # YAML 검토 전문가
│   └── operator-advisor.md    # Operator 패턴 전문가
├── templates/                 # 문서 템플릿
│   ├── service-doc.md         # 서비스/컴포넌트 문서 템플릿
│   ├── runbook.md             # 운영 런북 템플릿
│   └── incident-report.md     # 장애 보고서 템플릿
├── rules/                     # Claude 작성 규칙
│   ├── doc-writing.md         # 문서 작성 원칙
│   ├── k8s-conventions.md     # K8s 표준 관행
│   ├── security-checklist.md  # 보안 체크리스트
│   └── monitoring.md          # 모니터링 지침
├── components/                # 클러스터 구성 컴포넌트
├── objects/                   # 쿠버네티스 오브젝트/리소스
├── network/                   # 네트워킹 개념 및 기술
├── security/                  # 보안 심화
├── deep-dive/                 # 특정 주제 심층 분석
└── operator-example/          # Go로 작성한 실제 동작 Operator 예제
```

---

## 커스텀 슬래시 명령어

| 명령어 | 설명 | 사용 예시 |
|--------|------|---------|
| `/new-doc` | 새 K8s 문서 생성 | `/new-doc objects/hpa` |
| `/new-runbook` | 새 런북 생성 | `/new-runbook 노드 드레인` |
| `/review-doc` | 문서 품질 검토 | `/review-doc objects/pod.md` |
| `/add-troubleshooting` | 트러블슈팅 케이스 추가 | `/add-troubleshooting CrashLoopBackOff` |
| `/search-kb` | 지식베이스 검색 | `/search-kb HPA 스케일링` |

---

## 파일 네이밍 규칙
- 폴더 내 파일명은 **리소스/컴포넌트 이름만** 사용합니다. 접두사 불필요.
- 예: `objects/pod.md`, `components/apiserver.md`, `network/cni-service-proxy.md`
- 복합 개념은 하이픈으로 연결: `pv-pvc.md`, `cni-service-proxy.md`

---

## 문서 작성 표준 구조
각 파일은 아래 섹션을 **##(H2)** 헤더로 구성합니다.

```
## 1. 개요 및 비유
(한 문장 정의 + 직관적인 비유)

## 2. 핵심 설명
(bullet point로 핵심 개념 3~5개)

## 3. YAML 적용 예시
(실제 동작하는 YAML 코드 블록)

## 4. 트러블슈팅
(실무에서 자주 겪는 문제 + 해결책)
```

---

## 언어 규칙
- 모든 문서는 **한국어**로 작성합니다.
- YAML/코드 내 주석도 한국어를 권장합니다.
- 기술 용어(영문)는 첫 등장 시 한국어 병기: `Deployment(디플로이먼트)`

---

## 커버리지 목표
쿠버네티스의 모든 핵심 리소스(`kubectl api-resources` 결과 기준)와 컨트롤 플레인 컴포넌트를 최소 1개의 문서로 커버합니다.

---

## 백로그 (추가 예정)

현재 추가 예정 항목 없음.
