## 1. 개요 및 비유
**API Server**는 쿠버네티스 클러스터의 '관문'이자 모든 통신의 '중심점'입니다. 사용자의 명령(kubectl)이나 내부 컴포넌트 간의 모든 요청은 이 서버를 거쳐야만 합니다.

💡 **비유하자면 '정부 종합 민원실의 창구 직원'과 같습니다.**
여러분이 "파드 하나 만들어줘"라고 요청(kubectl apply)하면, 창구 직원(API Server)이 서류를 검토(인증/인가)하고, 문제가 없으면 이를 장부(etcd)에 기록합니다. 창구 직원이 없으면 아무리 훌륭한 부서(Scheduler, Controller)가 있어도 업무 지시를 내릴 방법이 없습니다.

[Image of Kubernetes API server as a central hub for cluster communication]

## 2. 핵심 설명
* **중앙 허브:** 모든 컴포넌트(Scheduler, Controller, Kubelet 등)는 서로 직접 대화하지 않고, 오직 API Server를 통해서만 상태 정보를 주고받습니다.
* **보안 필터:** 요청이 들어오면 인증(Authentication), 인가(Authorization), 준수 컨트롤(Admission Control) 단계를 거쳐 안전한 요청인지 검토합니다.
* **유일한 DB 접근자:** 클러스터의 상태 저장소인 `etcd`와 직접 통신할 수 있는 유일한 컴포넌트입니다.

## 3. YAML 적용 예시 (API Server 감사 로그 설정)
API Server에 누가 어떤 요청을 보냈는지 기록하는 Audit Policy 설정 예시입니다. (보통 컨트롤 플레인 설정 파일에 포함됩니다.)

```yaml
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  # 파드에 대한 변경 사항은 상세히 기록(RequestResponse)
  - level: RequestResponse
    resources:
    - group: ""
      resources: ["pods"]
  # 그 외의 모든 요청은 메타데이터만 기록
  - level: Metadata
    omitStages:
      - "RequestReceived"
```

## 4. 트러블 슈팅
* **`kubectl` 명령 시 `The connection to the server localhost:8080 was refused` 에러:**
  * 로컬의 `kubeconfig` 파일이 없거나, API Server 파드가 죽어서 통신이 아예 안 되는 상태입니다. `sudo crictl ps` 등을 통해 `kube-apiserver` 컨테이너가 살아있는지 확인하세요.
* **API Server 부하 급증:**
  * 특정 파드나 모니터링 툴이 너무 자주 API를 호출(Polling)할 때 발생합니다. `Watch` 메커니즘을 제대로 쓰고 있는지, 권한 설정이 너무 포괄적이지 않은지 점검해야 합니다.
