---
description: Kubernetes 신규 문서를 스캐폴딩합니다. 사용법: /new-doc <카테고리> <주제>
---

아래 지침에 따라 Kubernetes 지식 베이스 문서를 새로 작성해 주세요.

## 입력 정보
- 사용자 입력: $ARGUMENTS
- 첫 번째 인자 = 카테고리 (components, objects, network, security, deep-dive)
- 두 번째 인자 이후 = 주제 (하이픈 구분)

## 파일 생성 규칙
1. 파일명: `{주제}.md` (소문자, 하이픈 구분)
2. 저장 위치: `{카테고리}/` 디렉토리
3. `rules/doc-writing.md`의 문서 작성 규칙 준수
4. `rules/k8s-conventions.md`의 코드 작성 규칙 준수
5. `rules/security-checklist.md`의 보안 체크리스트 통과

## 문서 구조 (반드시 아래 섹션 모두 포함)

```markdown
# {주제명}

## 1. 개요 및 비유
(한 문장 정의 + 💡 직관적인 비유)

## 2. 핵심 설명
### 2.1 동작 원리
### 2.2 YAML 적용 예시
### 2.3 Best Practice

## 3. 트러블슈팅
### 3.1 주요 이슈
### 3.2 자주 발생하는 문제

## 4. 모니터링 및 확인
(kubectl 명령어, 관련 지표)

## 5. TIP
```

문서 작성 완료 후 `CLAUDE.md`의 카테고리별 파일 목록에 항목을 추가해 주세요.
