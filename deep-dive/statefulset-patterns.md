## 1. 개요 및 비유

StatefulSet은 파드마다 **고유한 정체성(이름, DNS, 스토리지)**을 부여합니다. 상태가 있는 데이터베이스, 메시지 큐, 분산 스토리지에 적합합니다.

💡 **비유하자면 번호표를 받은 레스토랑 테이블과 같습니다.**
Deployment 파드는 자리가 바뀌어도 상관없는 서서 먹는 음식. StatefulSet 파드는 예약된 번호 테이블(0번, 1번, 2번)에만 앉아야 하는 정찬입니다.

---

## 2. StatefulSet vs Deployment 핵심 차이

```
Deployment:
  파드 이름: my-deploy-7d9f8b6c4-xk2pq (랜덤)
  스토리지: 없거나 공유
  네트워크: 임의 IP, 재시작 시 변경
  순서: 중요하지 않음 (병렬 시작/종료)

StatefulSet:
  파드 이름: mysql-0, mysql-1, mysql-2 (고정 인덱스)
  스토리지: 파드마다 전용 PVC (mysql-0 → data-mysql-0)
  네트워크: 안정적인 DNS (mysql-0.mysql-svc.default.svc.cluster.local)
  순서: 0→1→2 순서로 시작, 2→1→0 역순으로 종료
```

---

## 3. 핵심 동작 심층 분석

### 3.1 volumeClaimTemplates — 파드별 PVC 자동 생성

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: mysql
spec:
  serviceName: mysql-headless    # Headless Service 이름 (DNS 생성에 필요)
  replicas: 3
  selector:
    matchLabels:
      app: mysql
  template:
    spec:
      containers:
      - name: mysql
        image: mysql:8.0
        volumeMounts:
        - name: data
          mountPath: /var/lib/mysql
        - name: config
          mountPath: /etc/mysql/conf.d

  volumeClaimTemplates:          # 파드마다 PVC 자동 생성
  - metadata:
      name: data
    spec:
      accessModes: [ReadWriteOnce]
      storageClassName: gp3
      resources:
        requests:
          storage: 100Gi
  # → data-mysql-0, data-mysql-1, data-mysql-2 PVC 생성

# PVC는 StatefulSet 삭제 후에도 보존됨 (데이터 보호)
# 수동 삭제 필요: kubectl delete pvc data-mysql-0
```

### 3.2 Pod Management Policy

```yaml
spec:
  podManagementPolicy: OrderedReady   # 기본값
  # - 0→1→2 순서로 생성, 각 파드 Ready 후 다음 생성
  # - 종료는 2→1→0 역순
  # - 업데이트도 2→1→0 역순으로 롤링

  podManagementPolicy: Parallel       # 병렬 처리
  # - 모든 파드 동시 생성/삭제
  # - 순서가 중요하지 않고 빠른 시작이 필요한 경우
  # - 예: Kafka 브로커, Redis 클러스터
```

### 3.3 Update Strategy

```yaml
updateStrategy:
  type: RollingUpdate            # 기본값
  rollingUpdate:
    partition: 0                 # 0 이상 인덱스 파드만 업데이트
    # partition: 2 이면 mysql-2만 업데이트, mysql-0/1은 그대로
    # → 카나리 업데이트에 활용!
    maxUnavailable: 1

updateStrategy:
  type: OnDelete                 # 수동 업데이트
  # kubectl delete pod mysql-0 으로 직접 삭제해야만 업데이트됨
  # 업데이트 시점을 직접 제어하고 싶을 때 사용
```

---

## 4. 실전 패턴 — MySQL Primary/Replica 구성

```yaml
# 1. Headless Service (개별 파드 DNS 접근용)
apiVersion: v1
kind: Service
metadata:
  name: mysql-headless
spec:
  clusterIP: None
  selector:
    app: mysql
  ports:
  - port: 3306

---
# 2. 읽기용 Service (모든 파드로 LB)
apiVersion: v1
kind: Service
metadata:
  name: mysql-read
spec:
  selector:
    app: mysql
  ports:
  - port: 3306

---
# 3. 쓰기용 Service (Primary만)
apiVersion: v1
kind: Service
metadata:
  name: mysql-write
spec:
  selector:
    app: mysql
    role: primary              # 레이블로 Primary만 선택
  ports:
  - port: 3306

---
# 4. StatefulSet
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: mysql
spec:
  serviceName: mysql-headless
  replicas: 3
  template:
    spec:
      initContainers:
      # init 컨테이너로 Primary/Replica 역할 결정
      - name: init-mysql
        image: mysql:8.0
        command:
        - bash
        - -c
        - |
          # 파드 인덱스 추출 (mysql-0 → 0)
          [[ `hostname` =~ -([0-9]+)$ ]] && ordinal=${BASH_REMATCH[1]}

          if [[ $ordinal -eq 0 ]]; then
            # mysql-0은 Primary
            echo "server-id=1" > /etc/mysql/conf.d/server-id.cnf
            echo "log-bin=/var/log/mysql/mysql-bin.log" >> /etc/mysql/conf.d/server-id.cnf
            cp /mnt/config-map/primary.cnf /etc/mysql/conf.d/
          else
            # mysql-1, mysql-2는 Replica
            echo "server-id=$((100+$ordinal))" > /etc/mysql/conf.d/server-id.cnf
            cp /mnt/config-map/replica.cnf /etc/mysql/conf.d/
          fi

      # Replica는 Primary에서 데이터 복제 (최초 또는 재시작)
      - name: clone-mysql
        image: gcr.io/google-samples/xtrabackup:1.0
        command:
        - bash
        - -c
        - |
          [[ `hostname` =~ -([0-9]+)$ ]] && ordinal=${BASH_REMATCH[1]}
          [[ $ordinal -eq 0 ]] && exit 0  # Primary는 복제 불필요

          # 데이터가 이미 있으면 건너뜀
          [[ -d /var/lib/mysql/mysql ]] && exit 0

          # 이전 파드(mysql-(n-1))에서 백업 수신
          ncat --recv-only mysql-$((ordinal-1)).mysql-headless 3307 | \
            xbstream -x -C /var/lib/mysql
          xtrabackup --prepare --target-dir=/var/lib/mysql
```

---

## 5. PVC 관리 패턴

### 5.1 StatefulSet 스케일다운 시 PVC 보존

```bash
# StatefulSet을 0으로 스케일다운해도 PVC는 남음
kubectl scale statefulset mysql --replicas=0
kubectl get pvc
# data-mysql-0, data-mysql-1, data-mysql-2 모두 남아있음

# 다시 스케일업 시 기존 PVC에 자동 연결
kubectl scale statefulset mysql --replicas=3
# → mysql-0이 다시 data-mysql-0에 연결됨 (데이터 복구)

# PVC 수동 정리 (데이터 삭제 주의!)
for i in 0 1 2; do
  kubectl delete pvc data-mysql-$i
done
```

### 5.2 스토리지 확장 (PVC Resize)

```bash
# 개별 PVC 확장
kubectl patch pvc data-mysql-0 \
  -p '{"spec":{"resources":{"requests":{"storage":"200Gi"}}}}'

# volumeClaimTemplates 변경은 StatefulSet에 직접 반영 안 됨
# 새 파드에만 적용됨 → 기존 PVC는 수동으로 확장 필요

# 모든 PVC 일괄 확장
for i in $(kubectl get pvc -l app=mysql -o name); do
  kubectl patch $i -p '{"spec":{"resources":{"requests":{"storage":"200Gi"}}}}'
done
```

---

## 6. 데이터 마이그레이션 패턴

### 6.1 Blue-Green StatefulSet 마이그레이션

```bash
# 기존: mysql (3개 파드, 100Gi 스토리지)
# 목표: mysql-new (200Gi 스토리지, 새 StorageClass)

# 1. 새 StatefulSet 생성 (다른 이름)
kubectl apply -f mysql-new-statefulset.yaml

# 2. 데이터 복사 (Velero, mysqldump, etc.)
kubectl exec mysql-0 -- mysqldump -u root -p mydb > /tmp/backup.sql
kubectl exec mysql-new-0 -- mysql -u root -p mydb < /tmp/backup.sql

# 3. 복제 설정으로 실시간 동기화
# (기존 → 새로운 Primary로 복제)

# 4. Service selector 교체 (다운타임 거의 없음)
kubectl patch service mysql-write \
  -p '{"spec":{"selector":{"app":"mysql-new"}}}'

# 5. 검증 후 기존 StatefulSet 삭제
kubectl delete statefulset mysql
```

---

## 7. 트러블슈팅

* **파드가 Pending — PVC 바인딩 안 됨:**
  ```bash
  # PVC 상태 확인
  kubectl get pvc -l app=mysql

  # StorageClass가 올바른지
  kubectl get storageclass

  # 기존 PVC와 이름이 다른 경우 (StatefulSet 이름 변경 시)
  # data-mysql-old-0 은 있지만 data-mysql-0은 없음
  # → PVC 이름 변경 또는 기존 PVC 재사용 불가 (수동 처리 필요)
  ```

* **특정 파드만 업데이트 안 됨 (partition 설정):**
  ```bash
  kubectl get statefulset mysql -o jsonpath='{.spec.updateStrategy}'
  # partition 값 확인

  # 전체 업데이트 진행
  kubectl patch statefulset mysql \
    -p '{"spec":{"updateStrategy":{"rollingUpdate":{"partition":0}}}}'
  ```

* **StatefulSet 삭제했는데 파드가 남음:**
  ```bash
  # cascade 옵션 확인
  kubectl delete statefulset mysql --cascade=orphan
  # → StatefulSet 삭제되고 파드/PVC는 고아 상태로 남음

  # 올바른 삭제 (파드 포함)
  kubectl delete statefulset mysql   # cascade=foreground가 기본
  ```
