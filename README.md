# Open-Simulator

[![Go Report Card](https://goreportcard.com/badge/github.com/alibaba/open-simulator)](https://goreportcard.com/report/github.com/alibaba/open-simulator)
![workflow build](https://github.com/alibaba/open-simulator/actions/workflows/build.yml/badge.svg)

## Motivation
### 概念定义

**Open-Simulator** 是 K8s 下的**仿真调度组件**。用户准备一批待创建 Workload 资源，Workload 资源指定好资源配额、绑核规则、亲和性规则、优先级等，通过 **Open-Simulator 的仿真调度能力**可判断当前集群是否能够满足 Workload 资源，以及添加多少资源可保证资源部署成功。

原生 Kubernetes 缺少**仿真调度能力**，且社区并没有相关项目供参考。**Open-Simulator** 可解决资源规划问题，通过Workload 调度要求计算出最少物理资源数量，进而提高资源使用率，为用户节省物理成本和运维成本。

## Use Case

两类场景需要资源规划：

- **交付前**：评估产品最少物理资源，通过仿真系统计算出交付需要的特定规格节点数量、磁盘数量（类似朱雀系统）；
- **运行时**：用户新建 or 扩容 Workload，仿真调度系统会给出当前集群物理资源是否满足，并给出集群扩容建议（详细到扩容节点数）

## Run

### 使用
#### 添加节点

执行命令

真实集群: ./simon apply --kube-config=[kubeconfig文件目录] -f [待调度的yaml资源文件夹]
模拟集群: ./simon apply --cluster-config=[clusterconfig文件目录] -f [待调度的yaml资源文件夹]

Yaml文件夹可支持多级子目录，以区分资源类型，参考./example目录。目前支持以下资源类型，更多类型会在后续支持:

- Pod
- Node 
- Deployment 
- StatefulSet
- DaemonSet

执行后输出一个名为configmap-simon.yaml的文件，用以保存结果。

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: simulator-plan
  namespace: kube-system
data:
  Deployment: '{"vivo-test-namespace/suppress-memcache-lsr":["simulator-node1","simulator-node1","node3","node2"],"vivo-test-namespace/suppress-memcache-be":["simulator-node1","simulator-node1","node3","node2"]}'
  StatefulSet: '{"vivo-test-namespace/suppress-memcache-lsr":["simulator-node1","simulator-node1","node3","node2"],"vivo-test-namespace/suppress-memcache-be":["simulator-node1","simulator-node1","node3","node2"]}'
```

### 效果图

![](doc/images/simon.png)
## Deployment

> 以 MacBook 为例

### 步骤

```bash
# 克隆项目
mkdir $(GOPATH)/github.com/alibaba
cd $(GOPATH)/github.com/alibaba
git clone https://github.com/alibaba/open-simulator.git
cd open-simulator

# 安装minikube并运行
minikube start

# 拷贝 kubeconfig 文件到项目目录
cp ~/.kube/config  ./kubeconfig

# 项目编译及运行
make
bin/simon apply --kubeconfig=./kubeconfig -f ./example/application_example/simple_example_by_huizhi
```
