# k8s-basic 프로젝트 가이드

## 프로젝트 목적
Kubernetes의 모든 핵심 컴포넌트, 오브젝트, 네트워킹 개념을 한국어로 쉽게 정리한 학습 레포지토리입니다.

## 폴더 구조
```
k8s-basic/
├── components/   # 클러스터 구성 컴포넌트 (apiserver, etcd, kubelet 등)
├── objects/      # 쿠버네티스 오브젝트/리소스 (pod, deployment, service 등)
├── network/      # 네트워킹 개념 및 기술 (cni, ipvs, network-policy 등)
├── security/     # 보안 심화 (security-context, image-security, PSS, secrets 등)
└── deep-dive/    # 특정 주제 심층 분석 (container-runtime, scheduling, deployment-strategy 등)
```

## 파일 네이밍 규칙
- 폴더 내 파일명은 **리소스/컴포넌트 이름만** 사용합니다. 접두사 불필요.
- 예: `objects/pod.md`, `components/apiserver.md`, `network/cni-service-proxy.md`
- 복합 개념은 하이픈으로 연결: `pv-pvc.md`, `cni-service-proxy.md`

## 문서 작성 표준 구조
각 파일은 아래 섹션을 **##(H2)** 헤더로 구성합니다.

```
## 1. 개요 및 비유
(한 문장 정의 + 💡 직관적인 비유)

## 2. 핵심 설명
(bullet point로 핵심 개념 3~5개)

## 3. YAML 적용 예시
(실제 동작하는 YAML 코드 블록)

## 4. 트러블 슈팅
(실무에서 자주 겪는 문제 + 해결책)
```

## 언어
- 모든 문서는 **한국어**로 작성합니다.
- YAML/코드 내 주석도 한국어를 권장합니다.
- 기술 용어(영문)는 첫 등장 시 한국어 병기: `Deployment(디플로이먼트)`

## 커버리지 목표
쿠버네티스의 모든 핵심 리소스(`kubectl api-resources` 결과 기준)와 컨트롤 플레인 컴포넌트를 최소 1개의 문서로 커버합니다.
