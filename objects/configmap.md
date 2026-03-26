## 1. 개요 및 비유
**ConfigMap(컨피그맵)**은 애플리케이션을 구동할 때 필요한 환경 변수, 설정 파일, 커맨드라인 인수 등을 컨테이너 이미지와 분리하여 저장하는 쿠버네티스 오브젝트입니다.

💡 **비유하자면 '스마트폰 앱의 설정(Settings) 메뉴'와 같습니다.**
주니어 엔지니어 여러분, 카카오톡 앱(컨테이너 이미지) 자체를 다시 설치하지 않아도 알림 소리나 테마(ConfigMap)를 설정 메뉴에서 바꿀 수 있죠? 마찬가지로 개발 환경(Dev)과 운영 환경(Prod)에서 접속해야 할 DB 주소가 다를 때, 이미지를 매번 새로 빌드할 필요 없이 ConfigMap만 갈아 끼워주면 됩니다.

## 2. 핵심 설명
* **설정의 분리(Decoupling):** 소스 코드 내부에 하드코딩된 설정값을 외부로 빼내어 애플리케이션의 이식성(Portability)을 높여줍니다.
* **주입 방식:** 파드(Pod)에 ConfigMap을 주입하는 방법은 크게 두 가지입니다.
  1. **환경 변수(Environment Variables):** 컨테이너 런타임에 변수 형태로 주입.
  2. **볼륨 마운트(Volume Mount):** 설정 파일(`nginx.conf` 등) 자체를 컨테이너 내부의 특정 디렉터리에 파일 형태로 덮어쓰기.
* **제한 사항:** 기밀 데이터(비밀번호, 인증 키 등)는 ConfigMap 대신 반드시 **Secret** 오브젝트를 사용해야 합니다.

## 3. YAML 적용 예시 (환경 변수 주입)
애플리케이션의 로그 레벨과 동작 모드를 ConfigMap으로 정의하고, 디플로이먼트에서 이를 환경 변수로 불러오는 예시입니다.

```yaml
# 1. ConfigMap 정의
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: default
data:
  LOG_LEVEL: "INFO"
  APP_MODE: "production"

---
# 2. 파드(Deployment)에서 ConfigMap 참조
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 1
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
      - name: app-container
        image: my-app:1.0
        envFrom:
        - configMapRef:
            name: app-config # 위에서 만든 ConfigMap 전체를 환경변수로 주입
```

## 4. 트러블 슈팅
* **ConfigMap 내용을 수정했는데 파드에 반영되지 않음:**
  * 환경 변수(`envFrom`)로 주입한 ConfigMap은 파드가 **재시작(Restart)**되어야만 새로운 값을 읽어옵니다. (볼륨 마운트로 주입한 경우는 수 분 내로 자동 동기화되지만, 애플리케이션 자체가 설정 리로드 기능을 지원해야 합니다.)
  * 해결책: `kubectl rollout restart deployment <이름>` 명령어로 파드를 재시작해 주세요.
