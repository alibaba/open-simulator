# 사용자-매뉴얼

- [매뉴얼](#사용자-매뉴얼)
  - [클러스터 시뮬레이션](#클러스터-시뮬레이션)
    - [가상 클러스터 구축](#가상-클러스터-구축)
    - [기존 클러스터 복사](#기존-클러스터-복사)
  - [애플리케이션 배포 시뮬레이션](#애플리케이션-배포-시뮬레이션)
    - [공용응용](#공용응용)
    - [차트 어플리케이션](#차트-애플리케이션)

## 클러스터-시뮬레이션

> simon에 대한 명령줄 설명은 [링크](../commandline/simon.md)를 참조하십시오.

### 가상-클러스터-구축

[example/simon-config.yaml](../../example/simon-config.yaml) 파일을 편집하여 사용자 지정 클러스터를 설정합니다.

````yaml
apiVersion: simon/v1alpha1
kind: Config
metadata:
  name: simon-config
spec:
  cluster:
    customConfig: example/cluster/demo_1
````

.spec.cluster.customConfig 필드의 내용은 가상 클러스터를 구축하는 데 필요한 파일이 포함된 폴더 경로입니다.

- 클러스터 노드 정보. 노드 yaml 파일은 example/cluster/demo_1/nodes 폴더에 저장됩니다.
- 클러스터 초기 컨테이너 리소스
  - 정적 파드(kube-scheduler, kube-apiserver 등). Pod의 yaml 파일은 example/cluster/demo_1의 매니페스트 폴더에 저장됩니다.
  - 일반 자원. yaml 파일은 example/cluster/demo_1 폴더에 저장됩니다.

명령을 실행하여 시뮬레이션된 가상 클러스터를 확인합니다.

```bash
bin/simon apply -i -f example/simon-config.yaml
```

### 기존-클러스터-복사

[example/simon-config.yaml](../../example/simon-config.yaml) 파일을 편집하고 kubeconfig 파일 경로를 설정한다.

```yaml
apiVersion: simon/v1alpha1
kind: Config
metadata:
  name: simon-config
spec:
  cluster:
    kubeConfig: /root/.kube/config
```

.spec.cluster.kubeConfig 필드는 실제 k8s 클러스터의 kubeconfig 파일의 절대 경로로 채워집니다.

명령을 실행하여 복사된 가상 클러스터를 확인합니다.

```bash
bin/simon apply -i -f example/simon-config.yaml
```

## 애플리케이션-배포-시뮬레이션

### 일반 애플리케이션

[example/simon-config.yaml](../../example/simon-config.yaml) 파일을 수정합니다.

```yaml
apiVersion: simon/v1alpha1
kind: Config
metadata:
  name: simon-config
spec:
  cluster:
    customConfig: example/cluster/demo_1
  appList:
    - name: simple
      path: example/application/simple
    - name: complicated
      path: example/application/complicate
  newNode: example/newnode/demo_1
````

배포할 애플리케이션 yaml 파일을 준비하고(이 예에서는 파일이 example/application/simple 및 example/application/complicate 디렉터리에 저장됨) .spec.cluster.appList 필드를 배열 형식으로 입력합니다. .

동시에 가상 클러스터의 자원이 애플리케이션의 전개 조건에 맞지 않는 것을 방지하기 위해 .spec.cluster.newNode 필드에 추가할 노드를 준비해야 한다.

명령을 실행하여 애플리케이션이 가상 클러스터에 배포되었는지 확인합니다.

```bash
bin/simon apply -i -f example/simon-config.yaml
```

### 차트-애플리케이션

[example/simon-config.yaml](../../example/simon-config.yaml) 파일을 수정합니다.

```yaml
apiVersion: simon/v1alpha1
kind: Config
metadata:
  name: simon-config
spec:
  cluster:
    customConfig: example/cluster/demo_1
  appList:
    - name: yoda
      path: example/application/charts/yoda
      chart: true
  newNode: example/newnode/demo_1
```

배포할 애플리케이션 차트 파일을 준비하고(이 예에서 파일은 example/application/charts/yoda 디렉토리에 저장됨) .spec.cluster.appList 필드에 배열 형식으로 채우십시오. 차트 필드가 true로 설정됩니다(기본값은 false).

명령을 실행하여 차트 애플리케이션이 가상 클러스터에 배포되었는지 확인합니다.

```bash
bin/simon apply -i -f example/simon-config.yaml
```
