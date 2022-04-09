# 用户使用手册

- [用户使用手册](#用户使用手册)
  - [集群模拟](#集群模拟)
    - [构建虚拟集群](#构建虚拟集群)
    - [复制现有集群](#复制现有集群)
  - [模拟部署应用](#模拟部署应用)
    - [普通应用](#普通应用)
    - [Chart应用](#chart应用)

## 集群模拟

> simon 的命令行说明详见[链接](../commandline/simon.md)

### 构建虚拟集群

编辑 [example/simon-config.yaml](../../example/simon-config.yaml) 文件，设置自定义集群：

```yaml
apiVersion: simon/v1alpha1
kind: Config
metadata:
  name: simon-config
spec:
  cluster:
    customConfig: example/cluster/demo_1
```

.spec.cluster.customConfig 字段内容为一个文件夹路径，其中包含了构建虚拟集群必要的文件:

- 集群节点信息。节点 yaml 文件存放在 example/cluster/demo_1/nodes 文件夹中
- 集群初始容器资源
  - 静态 Pod（如 kube-scheduler、kube-apiserver 等）。 Pod 的 yaml 文件存放在 example/cluster/demo_1 的 manifests 文件夹中
  - 一般资源。yaml 文件存放在 example/cluster/demo_1 文件夹中

执行命令，可看到模拟出的虚拟集群。

```bash
bin/simon apply -i -f example/simon-config.yaml
```

### 复制现有集群

编辑 [example/simon-config.yaml](../../example/simon-config.yaml) 文件，设置 kubeconfig 文件路径。

```yaml
apiVersion: simon/v1alpha1
kind: Config
metadata:
  name: simon-config
spec:
  cluster:
    kubeConfig: /root/.kube/config
```

.spec.cluster.kubeConfig 字段填入真实 k8s 集群的 kubeconfig 文件绝对路径。

执行命令，可看到复制出的虚拟集群。

```bash
bin/simon apply -i -f example/simon-config.yaml
```

## 模拟部署应用

### 普通应用

编辑 [example/simon-config.yaml](../../example/simon-config.yaml) 文件。

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
```

准备好待部署的应用 yaml 文件（本例中文件保存在 example/application/simple 和 example/application/complicate 目录），以数组形式填入到 .spec.cluster.appList 字段。

同时为防止虚拟集群的资源不满足应用的部署条件，需要准备一个待添加节点，在 .spec.cluster.newNode 字段。

执行命令，可看到应用部署在虚拟集群中。

```bash
bin/simon apply -i -f example/simon-config.yaml
```

### Chart应用

编辑 [example/simon-config.yaml](../../example/simon-config.yaml) 文件。

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

准备好待部署的应用 Chart 文件（本例中文件保存在 example/application/charts/yoda 目录），以数组形式填入到 .spec.cluster.appList 字段，注意 chart 字段设置为 true（默认为false）。

执行命令，可看到 Chart 应用部署在虚拟集群中。

```bash
bin/simon apply -i -f example/simon-config.yaml
```