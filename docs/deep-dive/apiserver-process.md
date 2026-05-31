## 1. 개요

Kubernetes는 **선언적 요구 사항(Desired State)**과 **현재 상태(Actual State)**를 일치시키는 거대한 제어 루프(Control Loop)의 집합입니다. 이 문서에서는 API Server가 데이터를 처리하는 방식과 Controller Manager가 어떻게 상태를 유지하는지 심층적으로 다룹니다.

---

## 2. 설명

### 2.1 API Server의 리소스 처리 과정

모든 요청은 API Server를 거치며, 다음과 같은 엄격한 단계를 따릅니다.

1. **Authentication & Authorization**: "누구인가? 권한이 있는가?"
2. **Mutating Admission Controller**: 요청된 리소스의 내용을 강제로 수정 (예: Sidecar 주입)
3. **Object Schema Validation**: 데이터 형식이 올바른지 확인
4. **Validating Admission Controller**: 최종 정책 위반 여부 확인
5. **etcd Persistence**: 최종 상태를 etcd에 저장

### 2.2 실무 적용 코드 (Custom Controller 개념의 Helm/Yaml)

실무에서는 기본 컨트롤러 외에도 특정 상태를 강제하기 위해 **Policy Engine(Kyverno/OPA)**을 사용합니다. 아래는 모든 Pod에 특정 레이블을 강제하는 Mutating Webhook의 로직을 대체하는 Kyverno 예시입니다.


```yaml
# enforced-labels-policy.yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: enforce-team-label
spec:
  rules:
    - name: check-team-label
      match:
        any:
        - resources:
            kinds:
              - Pod
      validate:
        message: "The 'team' label is required for all pods."
        pattern:
          metadata:
            labels:
              team: "?*" # team 레이블이 반드시 존재해야 함
```

### 2.3 보안(Security) 및 비용(Cost) Best Practice

- **보안 (Admission Control)**: `Privileged` 컨테이너 생성을 원천 차단하기 위해 `Pod Security Admission`을 반드시 활성화하세요.
- **비용 (API Server Load)**: 대규모 클러스터(노드 500대 이상)에서는 특정 컨트롤러의 과도한 `LIST` 호출이 etcd 부하를 일으킵니다. 반드시 **Informer의 Cache**를 활용하여 API Server 부하를 줄여야 합니다.

---

## 3. 트러블슈팅

### 3.1 Resource Version 충돌 (409 Conflict)

Kubernetes는 동시성 제어를 위해 **Optimistic Concurrency Control**을 사용합니다.

- **현상**: 두 개의 클라이언트가 동시에 동일 리소스를 수정하려 할 때 발생.
- **해결**: 클라이언트는 리소스를 다시 `GET` 하여 최신 `resourceVersion`을 획득한 후 `UPDATE`를 재시도해야 합니다. (Exponential Backoff 전략 필수)

### 3.2 Orphaned Resources (고아 리소스)

- **현상**: 부모 리소스(Deployment)는 삭제되었는데 자식(ReplicaSet)이 남는 경우.
- **해결**: `Garbage Collector` 컨트롤러의 동작을 확인하세요. 삭제 시 `propagationPolicy`를 `Foreground` 혹은 `Background`로 명시하여 연쇄 삭제를 보장할 수 있습니다.

---

## 4. 참고자료

- [Kubernetes Internals - Architecture](https://kubernetes.io/docs/concepts/overview/components/)
- [The Mechanics of Kubernetes Admission Controllers](https://kubernetes.io/blog/2019/03/21/a-guide-to-kubernetes-admission-controllers/)
- [API Server Concurrency Control (Optimistic Locking)](https://www.google.com/search?q=https://kubernetes.io/docs/reference/using-api/api-concepts/%23efficient-detection-of-changes)

---

## 5. TIP

- **kubectl debug**: 실행 중인 Pod에 문제가 생겼을 때, `kubectl debug` 명령어를 통해 Ephemeral Container를 삽입하여 내부 프로세스를 직접 조사할 수 있습니다.
- **Dry-run**: 운영 환경에 적용하기 전 `kubectl apply --dry-run=server`를 통해 Admission Controller가 해당 요청을 거부하는지 미리 테스트하는 습관을 들이세요.
- **Leads Election**: 커스텀 컨트롤러 제작 시, 고가용성을 위해 `leaderelection` 기능을 활성화하여 여러 인스턴스 중 하나만 활성 상태를 유지하도록 구성하세요.
