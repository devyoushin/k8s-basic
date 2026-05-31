# Runbook: {작업명}

> **분류**: {정기 PM | 장애 대응 | 업그레이드 | 스케일}
> **대상**: {리소스 타입/클러스터명}
> **작성일**: {YYYY-MM-DD}
> **예상 소요 시간**: {N분}
> **영향 범위**: {무중단 | 순단 N초 | 서비스 중단}

---

## 사전 체크리스트

- [ ] 작업 시간대 확인 (트래픽 낮은 시간대 권장)
- [ ] PodDisruptionBudget 확인
- [ ] 롤백 방법 숙지
- [ ] kubectl 접근 권한 확인

---

## 환경 변수 설정

```bash
export KUBECONFIG=<PATH>
export NAMESPACE=<NAMESPACE>
export TARGET_DEPLOYMENT=<DEPLOYMENT_NAME>
```

---

## Step 1: 사전 상태 확인

```bash
kubectl get pods -n $NAMESPACE
kubectl get nodes
```

---

## Step 2: {작업 내용}

```bash
kubectl ...
```

**확인 포인트**:
- {확인할 사항}

---

## Step 3: 완료 확인

```bash
kubectl rollout status deployment/$TARGET_DEPLOYMENT -n $NAMESPACE
```

**성공 기준**:
- [ ] 모든 Pod Ready
- [ ] 이벤트 오류 없음

---

## 롤백 절차

```bash
kubectl rollout undo deployment/$TARGET_DEPLOYMENT -n $NAMESPACE
```

---

## 모니터링 포인트

작업 완료 후 **15분간** 아래 지표 모니터링:

| 지표 | 정상 범위 | 이상 기준 |
|------|---------|---------|
| Pod Ready 비율 | 100% | <100% |
| Restart Count | 0 증가 | 증가 시 즉시 확인 |
