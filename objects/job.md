## 1. 개요 및 비유
**Job(잡)**은 하나 이상의 파드를 실행하여 지정한 작업이 성공적으로 완료(Completed)되도록 보장하는 컨트롤러입니다. CronJob의 기반이 되는 오브젝트이기도 합니다.

💡 **비유하자면 '배달 기사에게 맡긴 택배'와 같습니다.**
"이 택배를 목적지에 성공적으로 배달하면 임무 완료"라고 지시하는 것과 같습니다. 배달 도중 기사(파드)가 사고가 나더라도, Job이 다른 기사를 보내서 반드시 배달이 완료되게 만듭니다. Deployment처럼 계속 실행되는 것이 아니라, 완료 후 파드는 `Completed` 상태로 남습니다.

## 2. 핵심 설명
* **완료 보장:** 파드가 실패하면 `backoffLimit` 횟수만큼 재시도합니다. (기본값 6)
* **병렬 실행:** `parallelism` 설정으로 여러 파드를 동시에 실행해 작업을 빠르게 처리할 수 있습니다.
* **완료 조건:**
  * `completions`: 성공적으로 완료해야 하는 파드 총 수 (기본값 1)
  * `parallelism`: 동시에 실행할 파드 수 (기본값 1)
* **restartPolicy:** Job 파드는 반드시 `Never` 또는 `OnFailure`로 설정해야 합니다. (`Always`는 Job에 사용 불가)
  * `Never`: 실패 시 새 파드를 만들어 재시도 (이전 파드 로그 보존)
  * `OnFailure`: 실패 시 같은 파드를 재시작

## 3. YAML 적용 예시

### 단순 Job (한 번 실행)
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: db-migration
spec:
  backoffLimit: 3  # 최대 3번 재시도
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: migrator
        image: my-app:1.0
        command: ["python", "manage.py", "migrate"]
        env:
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: db-secret
              key: url
```

### 병렬 Job (대량 작업 분산 처리)
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: image-processor
spec:
  completions: 100   # 총 100개 작업을 완료해야 함
  parallelism: 10    # 동시에 10개 파드가 실행됨
  backoffLimit: 5
  template:
    spec:
      restartPolicy: OnFailure
      containers:
      - name: processor
        image: image-processor:1.0
        env:
        - name: TASK_INDEX
          valueFrom:
            fieldRef:
              fieldPath: metadata.annotations['batch.kubernetes.io/job-completion-index']
```

**유용한 명령어:**
```bash
# Job 상태 확인
kubectl get jobs

# Job이 만든 파드 로그 확인
kubectl logs -l job-name=db-migration

# Job 수동 실행 (kubectl create)
kubectl create job manual-run --from=cronjob/daily-backup
```

## 4. 트러블 슈팅
* **Job이 완료 안 되고 계속 파드를 생성함:**
  * 파드가 계속 실패하고 있는 것입니다. `kubectl describe job <잡명>` 으로 이벤트를 확인하고, `kubectl logs <파드명>` 으로 오류 로그를 분석하세요.
  * `backoffLimit`에 도달하면 Job은 `Failed` 상태로 종료됩니다.
* **완료된 Job 파드들이 클러스터에 계속 쌓임:**
  * `ttlSecondsAfterFinished` 필드를 설정하면 완료 후 지정된 시간(초)이 지나면 자동 삭제됩니다.
  ```yaml
  spec:
    ttlSecondsAfterFinished: 3600  # 완료 1시간 후 자동 삭제
  ```
* **Job이 무한정 실행되는 것을 막으려면:**
  * `activeDeadlineSeconds` 필드로 Job의 최대 실행 시간을 제한할 수 있습니다.
