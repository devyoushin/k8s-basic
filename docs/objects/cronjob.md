## 1. 개요 및 비유
**CronJob(크론잡)**은 정해진 시간이나 주기에 따라 일회성 작업(Job)을 생성하고 실행하는 역할을 합니다. 백업, 리포트 생성, 임시 데이터 정리 등에 사용됩니다.

💡 **비유하자면 '특정 시간에만 울리는 스마트 알람 시계'와 같습니다.**
일반적인 Deployment가 24시간 내내 켜져 있는 '편의점 알바생'이라면, CronJob은 "매일 새벽 2시에 일어나서 가게 바닥만 청소하고 다시 자라"고 예약해 두는 '새벽 청소 용역'입니다. 작업이 끝나면 컴퓨터 자원(CPU/메모리)을 즉시 반환하므로 비용이 절약됩니다.

## 2. 핵심 설명
* **Job 컨트롤러 기반:** CronJob 자체는 파드를 직접 만들지 않습니다. 설정된 시간이 되면 **Job**이라는 하위 오브젝트를 만들고, 이 Job이 파드를 띄워 작업을 수행한 뒤 완료(`Completed`) 상태로 만듭니다.
* **크론 표현식 (Cron Expression):** 리눅스의 `cron`과 동일하게 `분 시 일 월 요일` 포맷(예: `0 2 * * *`)을 사용합니다. 주의할 점은 K8s 컨트롤 플레인의 기본 시간대(Timezone)인 **UTC 기준**으로 동작한다는 것입니다. (K8s 1.27부터는 `timeZone` 필드 지원)
* **동시성 정책 (Concurrency Policy):** 이전 작업이 아직 안 끝났는데 다음 작업 주기가 도래했을 때 어떻게 할지 결정합니다. `Allow`(동시에 여러 개 띄움), `Forbid`(건너뜀), `Replace`(기존 작업 취소하고 새 작업 시작).

## 3. YAML 적용 예시 (주기적인 배치 작업)
매일 한국 시간(KST) 새벽 2시에 쉘 스크립트를 실행하여 특정 작업을 수행하는 예시입니다.

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: daily-backup
spec:
  schedule: "0 2 * * *" # 매일 새벽 2시에 실행
  timeZone: "Asia/Seoul" # K8s 1.27+ 이상 지원
  concurrencyPolicy: Forbid # 이전 작업이 밀려있으면 이번 작업은 건너뜀
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure # 실패 시에만 파드 재시작
          containers:
          - name: backup-worker
            image: busybox
            command:
            - /bin/sh
            - -c
            - "echo 'Starting backup...'; sleep 30; echo 'Backup Done!'"
```

## 4. 트러블 슈팅
* **시간이 지났는데 작업이 실행되지 않음:**
  * 가장 먼저 확인할 것은 **시간대(Timezone) 착각**입니다. `timeZone` 필드를 명시하지 않았다면 UTC(한국 시간보다 9시간 느림) 기준으로 동작하므로, 엉뚱한 낮 시간에 실행되고 있을 확률이 높습니다.
* **완료된 파드(Job)가 계속 쌓여서 클러스터가 지저분해짐:**
  * 성공/실패한 작업 이력을 몇 개나 남길지 지정하는 `successfulJobsHistoryLimit` (기본 3)와 `failedJobsHistoryLimit` (기본 1) 필드를 조절하여 자동으로 청소되도록 설정해야 합니다.
