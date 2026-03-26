## 1. 개요 및 비유
**StatefulSet(스테이트풀셋)**은 데이터베이스처럼 상태(State)를 가지는 애플리케이션을 위한 컨트롤러입니다. 각 파드에 고유한 식별자(순서 번호)와 안정적인 스토리지를 부여합니다.

💡 **비유하자면 '번호판이 있는 지정석'과 같습니다.**
Deployment의 파드들이 번호가 없는 공용 좌석이라면, StatefulSet의 파드들은 `mysql-0`, `mysql-1`, `mysql-2` 처럼 고유 번호가 새겨진 지정석입니다. 좌석 주인(파드)이 바뀌어도 번호(이름)와 사물함(PVC)은 그대로 유지됩니다.

## 2. 핵심 설명
* **Deployment와의 차이점:**

| 항목 | Deployment | StatefulSet |
|---|---|---|
| 파드 이름 | 무작위 (`web-6d8f...`) | 순서 보장 (`mysql-0`, `mysql-1`) |
| 스케일링 순서 | 동시에 | 순차적 (0→1→2 생성, 역순 삭제) |
| 스토리지 | 공유 또는 개별 | 파드별 고유 PVC 자동 생성 |
| DNS | 없음 | 파드별 안정적인 DNS 제공 |

* **Headless Service 필요:** StatefulSet은 반드시 `clusterIP: None`인 Headless Service와 함께 사용합니다. 이를 통해 각 파드에 `<파드명>.<서비스명>.<네임스페이스>.svc.cluster.local` 형태의 개별 DNS가 생깁니다.
* **volumeClaimTemplates:** 각 파드마다 자동으로 PVC를 생성하는 템플릿입니다. 파드가 삭제되어도 PVC는 남아서 재생성 시 동일한 데이터를 마운트합니다.

## 3. YAML 적용 예시 (MySQL 클러스터)

```yaml
# Headless Service (파드별 개별 DNS 제공)
apiVersion: v1
kind: Service
metadata:
  name: mysql
  namespace: default
spec:
  clusterIP: None  # Headless 설정
  selector:
    app: mysql
  ports:
  - port: 3306

---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: mysql
spec:
  serviceName: "mysql"  # 위 Headless Service 이름과 일치해야 함
  replicas: 3
  selector:
    matchLabels:
      app: mysql
  template:
    metadata:
      labels:
        app: mysql
    spec:
      containers:
      - name: mysql
        image: mysql:8.0
        env:
        - name: MYSQL_ROOT_PASSWORD
          valueFrom:
            secretKeyRef:
              name: mysql-secret
              key: root-password
        ports:
        - containerPort: 3306
        volumeMounts:
        - name: data        # volumeClaimTemplates의 이름과 일치
          mountPath: /var/lib/mysql
  volumeClaimTemplates:     # 파드별 PVC 자동 생성
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: gp3
      resources:
        requests:
          storage: 20Gi
```

생성되면 다음 파드들이 순서대로 생깁니다:
- `mysql-0` → `mysql-0.mysql.default.svc.cluster.local`
- `mysql-1` → `mysql-1.mysql.default.svc.cluster.local`
- `mysql-2` → `mysql-2.mysql.default.svc.cluster.local`

## 4. 트러블 슈팅
* **파드가 순서대로 안 뜨고 멈춤:**
  * `mysql-0`이 Ready 상태가 되어야 `mysql-1`이 시작됩니다. 이전 파드가 Ready가 안 되는 원인(PVC 마운트 실패, 앱 초기화 오류)을 먼저 해결해야 합니다.
* **StatefulSet을 삭제했는데 PVC가 남아있음:**
  * 의도된 동작입니다. 데이터 손실을 방지하기 위해 StatefulSet 삭제 시 PVC는 자동 삭제되지 않습니다. 데이터가 필요없다면 PVC를 수동으로 삭제하세요.
  ```bash
  kubectl delete pvc -l app=mysql
  ```
* **특정 파드만 재시작하고 싶을 때:**
  * `kubectl delete pod mysql-1` 로 파드를 삭제하면 StatefulSet이 동일한 이름의 파드를 다시 만들고 같은 PVC를 마운트합니다.
