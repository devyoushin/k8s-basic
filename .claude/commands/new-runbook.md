---
description: K8s 운영 Runbook을 생성합니다. 사용법: /new-runbook <리소스타입> <작업명>
---

`$ARGUMENTS`를 파싱하여 Kubernetes 운영 Runbook을 생성합니다.
- 첫 번째 인자: 대상 리소스 (pod, deployment, node, pvc, secret 등)
- 나머지 인자: 작업명

## 파일 생성 규칙
1. 파일명: `runbook-{리소스}-{작업명}.md`
2. 저장 위치: 적합한 카테고리 디렉토리
3. `templates/runbook.md` 템플릿 기반으로 작성

## 작성 시 필수 포함 사항
- **사전 체크리스트**: 최소 4개 항목
- **환경 변수 설정**: `export` 형식으로 명시
- **단계별 kubectl 명령어**: 복붙 즉시 실행 가능
- **확인 명령어**: 각 스텝 완료 후 검증 방법
- **롤백 절차**: 되돌릴 수 있는 명령어 포함
- **모니터링 포인트**: 작업 후 감시할 지표
