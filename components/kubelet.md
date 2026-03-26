## 1. 개요 및 비유
**Kubelet**은 각 노드에 상주하는 '현장 소장'입니다. 컨트롤 플레인의 명령을 받아 실제로 컨테이너를 띄우고 상태를 보고합니다.

💡 **비유하자면 '공사 현장의 십장(현장 작업 반장)'과 같습니다.**
본사(Control Plane)에서 "이번 층에 방 3개(Pod 3개) 만들어"라는 설계도(PodSpec)를 보내주면, 현장에서 직접 자재를 나르고 인부들을 시켜 컨테이너를 올립니다. 그리고 주기적으로 본사에 "지시하신 대로 방 3개 잘 돌아가고 있습니다"라고 보고(Health Check)를 올립니다.

## 2. 핵심 설명
* **PodSpec 준수:** API Server로부터 받은 파드 명세서(`PodSpec`)를 읽어 컨테이너 런타임(Docker/containerd)에 컨테이너 생성을 요청합니다.
* **상태 보고:** 노드와 파드의 상태를 주기적으로 체크하여 API Server에 업데이트합니다.
* **Liveness/Readiness Probe:** 컨테이너가 살아있는지, 서비스할 준비가 되었는지 직접 검사합니다.

## 3. YAML 적용 예시 (Kubelet 설정 파일)
노드 자원이 부족할 때 파드를 쫓아내는 기준(Eviction) 등을 설정합니다.

```yaml
# /var/lib/kubelet/config.yaml 예시
apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
evictionHard:
  memory.available: "100Mi" # 메모리가 100MB 미만으로 떨어지면 파드 정리 시작
  nodefs.available: "10%"   # 디스크 잔량이 10% 미만이면 정리 시작
```

## 4. 트러블 슈팅
* **노드 상태가 `NotReady`인 경우:**
  * Kubelet 프로세스가 죽었거나, API Server와 통신이 끊긴 상태입니다. 노드에 접속하여 `systemctl status kubelet` 명령어로 프로세스 생존 여부를 확인하세요.
* **`NodeHasDiskPressure` 경고:**
  * Kubelet이 노드의 디스크가 꽉 찼음을 감지한 것입니다. 로그 파일을 지우거나 디스크 용량을 늘려야 합니다.
