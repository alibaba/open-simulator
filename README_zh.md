# Open-Simulator

[![Go Report Card](https://goreportcard.com/badge/github.com/alibaba/open-simulator)](https://goreportcard.com/report/github.com/alibaba/open-simulator)
![workflow build](https://github.com/alibaba/open-simulator/actions/workflows/build.yml/badge.svg)

[English](./README.md) | 简体中文

## 介绍

Open-Simulator 是一个服务于Kubernetes集群部署的**集群模拟组件**。Open-Simulator 主要通过模拟**Kube-Controller-Manager**和**Kube-Scheduler**组件的能力，以生成源于 [workload](https://kubernetes.io/zh/docs/concepts/workloads/) 资源的Pod实例，并基于调度策略来调度Pod，达到模拟在真实环境中部署的效果，最后用户根据部署情况得到一个合理的部署方案，亦或者自己酌情制定。

## 使用场景

Open-Simulator 意图解决 Kubernetes 中棘手的**容量规划**问题：

- 集群规格计算：根据现有的服务器规格（CPU核数、内存、磁盘）以及应用部署文件（包含了指定副本数、亲和性规则、资源申请量的各种 workloads ），计算出成功安装集群所需要的**最少节点数量**；
- 应用部署模拟：在已运行的 Kubernetes 集群中，模拟待部署的应用是否可以成功部署；若集群规模不满足部署情况，则给出集群最少扩容建议，以解决 All-or-Nothing 应用调度的问题；
- 空闲节点清理：在已运行的 Kubernetes 集群中，根据自定义规则筛选并下线空闲节点。

通过合理的**容量规划**，用户可减少人力交付成本和运维成本，并可提高集群资源整体利用率。

## ✅ 特性

- [x] 支持创建任意规格的 K8s 集群
- [x] 支持部署 Workload ，种类包含
  - [x] Deployment
  - [x] StatefulSet
  - [x] Daemonset
  - [x] Job
  - [x] CronJob
  - [x] Pod
- [x] 支持模拟 Kube-Scheduler 调度并给出应用部署拓扑结果
- [x] 支持自动添加节点以满足应用成功部署
- [x] 支持模拟 [Open-Local](https://github.com/alibaba/open-local) 存储调度
- [x] 支持解析 Helm Chart
- [x] 支持设置集群资源水位
- [x] 支持设置 Workload 部署顺序
- [ ] 支持解析 CR 资源
- [ ] 支持处理 PV/PVC 资源
- [ ] 支持清理空闲节点

## 🚀 快速开始

### 项目构建

```bash
mkdir -p $(GOPATH)/github.com/alibaba
cd $(GOPATH)/github.com/alibaba
git clone git@github.com:alibaba/open-simulator.git
cd open-simulator
make
```

### 运行

```bash
# 交互模式
bin/simon apply -i -f example/simon-config.yaml
```

其中配置文件 [example/simon-config.yaml](example/simon-config.yaml) 如下所示：

```yaml
apiVersion: simon/v1alpha1
kind: Config
metadata:
  name: simon-config
spec:
  # cluster: 导入生成初始集群的配置文件(以下皆为文件路径)
  #   customConfig: 自定义集群的配置文件
  #   kubeConfig: 真实集群的kube-config文件
  #   以上两者取其一
  cluster:
    customConfig: example/cluster/demo_1

  # appList: 导入需部署的应用
  # 支持chart和非chart文件；文件格式可为文件夹或者压缩包格式。
  # 多个应用时，部署顺序为配置顺序
  #   name: 应用名称
  #   path: 应用文件
  #   chart: 缺省；若chart指定为true，则表示应用文件为chart文件，若为false或者不指定chart则为非chart文件
  appList:
    - name: yoda
      path: example/application/charts/yoda
      chart: true
    - name: simple
      path: example/application/simple
    - name: complicated
      path: example/application/complicate
    - name: open_local
      path: example/application/open_local
    - name: more_pods
      path: example/application/more_pods

  # newNode: 导入调整集群规模的节点配置文件，节点规格可根据需求任意指定。目前只支持配置一个节点
  newNode: example/newnode
```

运行效果图：

![](./docs/images/simon.png)

## 许可证

[Apache 2.0 License](LICENSE)