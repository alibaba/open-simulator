# Open-Simulator

[![Go Report Card](https://goreportcard.com/badge/github.com/alibaba/open-simulator)](https://goreportcard.com/report/github.com/alibaba/open-simulator)
![workflow build](https://github.com/alibaba/open-simulator/actions/workflows/build.yml/badge.svg)

English | [简体中文](./README_zh.md)

## Introduction

Open-simulator is a **cluster simulator** for Kubernetes. With the simulation capability of Open-Simulator, users can create a fake Kubernetes cluster and deploy [workloads](https://kubernetes.io/zh/docs/concepts/workloads/) on it. Open-Simulator will simulate the [kube-controller-manager](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-controller-manager/) to create pods for the workloads, and simulate the [kube-scheduler](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-scheduler/) to assign pods to the appropriate nodes.

## Use Case

Open-Simulator intends to reduce the labor costs in the delivery phase and operation and maintenance costs after the delivery phase, improve the overall utilization of cluster resources by solving the following thorny problems in Kubernetes:

- **Capacity Planning**: according to the existing server specifications (including the number of CPU cores, size of memory, capacity of disk, etc) and application workloads files (including the replicas, affinity rules, resource requirements, etc), calculate the minimum number of nodes required to install the cluster and its applications successfully.
- **Simulating Deploying Applications**: in the running kubernetes cluster, determine whether the applications can be deployed successfully at one time by simulating deploying applications in the fake mirror cluster. If the cluster size does not meet the resource requirements of applications, a minimum cluster expansion proposal is given to solve the All-or-Nothing problem of deploying applications;
- **Pods Migration**: in the running Kubernetes cluster, pods can be migrated between nodes according to the migration policy(such as scaling down cluster, defragmentation, etc).

## ✅ Feature

- [x] Support to create fake kubernetes clusters of any size you want
- [x] Support to deploy various workloads, including as follows:
  - [x] Deployment
  - [x] StatefulSet
  - [x] Daemonset
  - [x] ReplicaSet
  - [x] Job
  - [x] CronJob
  - [x] Pod
- [x] Support to simulate Kube-Scheduler and report the topology results of applications deployment
- [x] Support the automatic addition of fake nodes to deploy applications successfully
- [x] Support to simulate storage scheduling of [Open-Local](https://github.com/alibaba/open-local)
- [x] Support helm chart
- [x] Support setting the average resource utilization during capacity planning
- [x] Support the custom deployment order of multiple applications
- [ ] Support Custom Resource
- [ ] Topology-Aware Volume Scheduling
- [ ] Pods Migration

## 🚀 Quick start

### Build

```bash
mkdir -p $(GOPATH)/github.com/alibaba
cd $(GOPATH)/github.com/alibaba
git clone git@github.com:alibaba/open-simulator.git
cd open-simulator
make
```

### Run

```bash
# Interactive Mode
bin/simon apply -i -f example/simon-config.yaml
```

[example/simon-config.yaml](example/simon-config.yaml):

```yaml
apiVersion: simon/v1alpha1
kind: Config
metadata:
  name: simon-config
spec:
  # the file path to generate the fake cluster, select one of the following
  # cluster:
  #   customConfig: custom cluster file path
  #   kubeConfig: The kube-config file path of the real cluster
  cluster:
    customConfig: example/cluster/demo_1

  # list of applications to be deployed
  # for multiple applications, the order of deployment is the order of configuration in the list
  # appList:
  #   name: set name to distinguish applications conveniently
  #   path: the path of the application files
  #   chart: if the value of chart is specified as true, it means that the application is a chart; If false or not set, it is a non-chart
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

  # the specification of node to be added, which can be file path or folder path
  newNode: example/newnode
```

Preview

![](./docs/images/simon.png)

## License

[Apache 2.0 License](LICENSE)