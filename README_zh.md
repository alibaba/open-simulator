# Open-Simulator

[![Go Report Card](https://goreportcard.com/badge/github.com/alibaba/open-simulator)](https://goreportcard.com/report/github.com/alibaba/open-simulator)
![workflow build](https://github.com/alibaba/open-simulator/actions/workflows/build.yml/badge.svg)

[English](./README.md) | 简体中文

## 介绍

Open-Simulator 是 Kubernetes 下的**集群模拟组件**。通过 Open-Simulator 的模拟能力，用户可创建虚拟 Kubernetes 集群，并在其上部署 [Workload](https://kubernetes.io/zh/docs/concepts/workloads/) 资源。Open-Simulator 会模拟 [Kube-Controller-Manager](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-controller-manager/) 在虚拟集群中生成 Workload 资源的 Pod 实例，并模拟 [Kube-Scheduler](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-scheduler/) 对 Pod 进行调度。

## 使用场景

- 容量规划：根据现有服务器规格（包含CPU核数、内存、磁盘）以及应用部署文件（包含指定副本数、亲和性规则、资源申请量），规划出成功安装集群及其应用所需要的节点数量；
- 仿真调度：在已运行的 Kubernetes 集群中，判断待部署的应用是否可以一次性部署成功部署；若集群规模不满足部署要求，规划出需添加的节点数量，以解决 All-or-Nothing 应用调度问题；
- 容器迁移：在已运行的 Kubernetes 集群中，根据策略对 Pod 进行节点间迁移。未来考虑支持如下迁移策略：
  - 集群缩容
  - 碎片整理

通过解决如上问题，Open-Simulator 将减少人力交付成本和运维成本，并提高集群资源整体利用率。

## ✅ 特性

- 支持创建任意规格的 K8s 集群
- 支持按照指定顺序部署 Workloads
- 支持模拟 Kube-Scheduler 调度并给出应用部署拓扑结果
- 支持扩展调度算法
- 支持设置集群资源水位

## 用户手册

详见[文档](docs/user-guide/user-guide_zh_CN.md)

## 许可证

[Apache 2.0 License](LICENSE)