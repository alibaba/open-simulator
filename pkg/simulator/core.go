package simulator

import (
	"github.com/alibaba/open-simulator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	storagev1 "k8s.io/api/storage/v1"
)

type SimulateResult struct {
	UnscheduledPods []UnscheduledPod
	NodeStatus      []NodeStatus
}

type UnscheduledPod struct {
	Pod    *corev1.Pod
	Reason string
}

type NodeStatus struct {
	Node *corev1.Node
	Pods []*corev1.Pod
}

type ResourceTypes struct {
	Nodes                  []*corev1.Node
	Pods                   []*corev1.Pod
	DaemonSets             []*appsv1.DaemonSet
	StatefulSets           []*appsv1.StatefulSet
	Deployments            []*appsv1.Deployment
	ReplicationControllers []*corev1.ReplicationController
	ReplicaSets            []*appsv1.ReplicaSet
	Services               []*corev1.Service
	PersistentVolumeClaims []*corev1.PersistentVolumeClaim
	StorageClasss          []*storagev1.StorageClass
	PodDisruptionBudgets   []*policyv1beta1.PodDisruptionBudget
	Jobs                   []*batchv1.Job
	CronJobs               []*batchv1beta1.CronJob
}

type AppResource struct {
	Name     string
	Resource ResourceTypes
}

type Interface interface {
	RunCluster(cluster ResourceTypes) (*SimulateResult, error)
	ScheduleApp(AppResource) (*SimulateResult, error)
	Close()
}

// Simulate
// 参数
// 1. 由使用方自己生成 cluster 和 apps 传参
// 2. apps 将按照顺序模拟部署
// 3. 存储信息以 Json 形式填入对应的 Node 资源中
// 返回值
// 1. error 不为空表示函数执行失败
// 2. error 为空表示函数执行成功，通过 SimulateResult 信息获取集群模拟信息。其中 UnscheduledPods 表示无法调度的 Pods，若其为空表示模拟调度成功；NodeStatus 会详细记录每个 Node 上的 Pod 情况。
func Simulate(cluster ResourceTypes, apps []AppResource, opts ...Option) (*SimulateResult, error) {
	// init simulator
	sim, err := New(opts...)
	if err != nil {
		return nil, err
	}
	defer sim.Close()

	cluster.Pods, err = GetValidPodExcludeDaemonSet(cluster)
	if err != nil {
		return nil, err
	}
	for _, item := range cluster.DaemonSets {
		validPods, err := utils.MakeValidPodsByDaemonset(item, cluster.Nodes)
		if err != nil {
			return nil, err
		}
		cluster.Pods = append(cluster.Pods, validPods...)
	}

	var failedPods []UnscheduledPod
	// run cluster
	result, err := sim.RunCluster(cluster)
	if err != nil {
		return nil, err
	}
	failedPods = append(failedPods, result.UnscheduledPods...)

	// schedule pods
	for _, app := range apps {
		result, err = sim.ScheduleApp(app)
		if err != nil {
			return nil, err
		}
		failedPods = append(failedPods, result.UnscheduledPods...)
	}
	result.UnscheduledPods = failedPods

	return result, nil
}
