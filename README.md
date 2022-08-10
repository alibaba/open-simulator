# Open-Simulator

[![Go Report Card](https://goreportcard.com/badge/github.com/alibaba/open-simulator)](https://goreportcard.com/report/github.com/alibaba/open-simulator)
![workflow build](https://github.com/alibaba/open-simulator/actions/workflows/build.yml/badge.svg)

English | [简体中文](./README_zh.md) | [Korean](./README_ko.md)

## Introduction

Open-simulator is a **cluster simulator** for Kubernetes. With the simulation capability of Open-Simulator, users can create a fake Kubernetes cluster and deploy [workloads](https://kubernetes.io/zh/docs/concepts/workloads/) on it. Open-Simulator will simulate the [kube-controller-manager](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-controller-manager/) to create pods for the workloads, and simulate the [kube-scheduler](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-scheduler/) to assign pods to the appropriate nodes.

## Use Case

- **Capacity Planning**: plan out the number of nodes needed to install the cluster and deploy its applications successfully according to the existing server specifications (including the number of CPU cores, size of memory, capacity of disk, etc) and application workloads files (including the replicas, affinity rules, resource requirements, etc)
- **Simulating Deploying Applications**: determine whether the applications can be deployed successfully at one time by simulating deploying applications in the running kubernetes cluster. If the cluster size does not meet the resource requirements of applications, plan out the number of nodes to add
- **Pods Migration**: in the running Kubernetes cluster, pods can be migrated between nodes according to the migration policy(such as scaling down cluster, defragmentation, etc).

Open-Simulator intends to reduce the labor costs in the delivery phase and maintenance costs in production environment, improve the overall utilization of cluster resources by solving these thorny issues listed above.

## ✅ Feature

- Create fake kubernetes clusters of any size
- Deploy various workloads according to the custom order
- Simulate Kube-Scheduler and report the topology results of applications deployment
- Extend scheduling algorithm
- Set the average resource utilization during capacity planning

## User guide

More details [here](docs/user-guide/user-guide_zh_CN.md)

## License

[Apache 2.0 License](LICENSE)
