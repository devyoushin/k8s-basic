# AGENTS.md — k8s-basic Codex 작업 지침

이 저장소는 Kubernetes 개념과 운영 심화 지식 베이스입니다. Codex 작업 시 `CLAUDE.md`와 `docs/rules/`의 규칙을 동일하게 따릅니다.

## 공통 원칙

- 학습 문서는 `docs/` 아래에 둡니다.
- 실행 가능한 실습 코드는 `ops/` 아래에 둡니다.
- Kubernetes 예시는 `apiVersion`, `kind`, `metadata.namespace`를 명확히 작성합니다.
- 운영 문서는 관측, 트러블슈팅, rollback 관점을 포함합니다.

## Claude와의 싱크

- `CLAUDE.md`는 Claude용 상세 지침입니다.
- `AGENTS.md`는 Codex용 진입점입니다.
- 공통 규칙은 `docs/rules/`를 기준으로 유지합니다.

## 작업 체크리스트

- 기존 변경 확인
- YAML 문법 검사
- Go/operator 코드는 가능하면 `go test` 또는 `go test ./...`
- 링크 검사와 `git diff --check` 수행
