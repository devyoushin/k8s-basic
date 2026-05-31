## 1. 개요 및 비유
**etcd**는 쿠버네티스의 모든 설정 데이터와 클러스터의 현재 상태 정보가 저장되는 '공식 저장소'입니다. 고가용성을 지원하는 Key-Value 저장소입니다.

💡 **비유하자면 '클러스터의 비밀 금고 속 마스터 장부'와 같습니다.**
회사의 모든 기밀 사항, 직원 명단, 자산 현황이 이 장부(etcd)에 적혀 있습니다. 이 장부를 잃어버리면 아무리 서버가 쌩쌩 돌아가도 쿠버네티스는 자신이 누구인지, 어떤 파드를 돌려야 하는지 완전히 까먹게 됩니다.

## 2. 핵심 설명
* **일관성 중시:** 분산 시스템에서 데이터가 꼬이지 않도록 Raft 알고리즘을 사용하여 모든 노드가 동일한 데이터를 갖도록 보장합니다.
* **SSOT (Single Source of Truth):** 클러스터의 유일한 진실의 원천입니다. API Server만 이 금고에 접근할 수 있습니다.
* **백업 필수:** EKS 같은 관리형 서비스는 AWS가 백업해주지만, 직접 설치(Self-managed)했다면 반드시 주기적으로 스냅샷 백업을 해야 합니다.

## 3. YAML 적용 예시 (etcd 스냅샷 백업 명령어)
설정 파일은 아니지만, 엔지니어라면 반드시 외워야 할 백업 명령어입니다.

```bash
# etcdctl을 이용한 수동 백업 예시
ETCDCTL_API=3 etcdctl \
  --endpoints=[https://127.0.0.1:2379](https://127.0.0.1:2379) \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key \
  snapshot save /tmp/etcd-backup.db
```

## 4. 트러블 슈팅
* **디스크 I/O 레이턴시 에러:**
  * etcd는 쓰기 속도에 매우 민감합니다. 디스크 속도가 느리면 `etcd server is slow`라는 경고가 뜨며 클러스터 전체 응답이 느려집니다. 반드시 SSD(AWS라면 gp3 이상) 환경에서 구동하세요.
