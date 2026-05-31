# {주제명}

> **카테고리**: {components | objects | network | security | deep-dive}
> **관련 컴포넌트**: {관련 K8s 리소스}
> **작성일**: {YYYY-MM-DD}

---

## 1. 개요 및 비유

{한 문장 정의}

> 💡 **비유**: {직관적인 비유로 개념 설명}

---

## 2. 핵심 설명

### 2.1 동작 원리

{핵심 개념 설명}

### 2.2 YAML 적용 예시

```yaml
apiVersion:
kind:
metadata:
  name: <YOUR_NAME>
  namespace: <YOUR_NAMESPACE>
  labels:
    app: <YOUR_APP>
    version: v1
spec:
```

### 2.3 Best Practice

- {Best Practice 1}
- {Best Practice 2}

---

## 3. 트러블슈팅

### {증상 1}

**증상**: {에러 메시지 또는 현상}

**원인**: {근본 원인}

**해결 방법**:
```bash
kubectl ...
```

---

## 4. 모니터링 및 확인

```bash
# 상태 확인
kubectl get {리소스타입} -n <NAMESPACE>

# 상세 정보
kubectl describe {리소스타입} <NAME> -n <NAMESPACE>

# 이벤트 확인
kubectl get events -n <NAMESPACE> --sort-by='.lastTimestamp'
```

---

## 5. TIP

- {실무 팁 1}
- {실무 팁 2}
