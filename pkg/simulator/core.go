package simulator

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/alibaba/open-simulator/pkg/utils"
	"k8s.io/apimachinery/pkg/watch"
	utiltrace "k8s.io/utils/trace"
)

// 仿真结果
type SimulateResult struct {
	UnscheduledPods []UnscheduledPod `json:"unscheduledPods"`
	NodeStatus      []NodeStatus     `json:"nodeStatus"`
}

// 无法成功调度的 Pod 信息
type UnscheduledPod struct {
	Pod    *corev1.Pod `json:"pod"`
	Reason string      `json:"reason"`
}

// 已成功调度的 Pod 信息
type NodeStatus struct {
	// 节点信息
	Node *corev1.Node `json:"node"`
	// 该节点上所有 Pod 信息
	Pods []*corev1.Pod `json:"pods"`
}

type ResourceTypes struct {
	Nodes                  []*corev1.Node
	Pods                   []*corev1.Pod
	DaemonSets             []*appsv1.DaemonSet
	StatefulSets           []*appsv1.StatefulSet
	Deployments            []*appsv1.Deployment
	ReplicaSets            []*appsv1.ReplicaSet
	Services               []*corev1.Service
	PersistentVolumeClaims []*corev1.PersistentVolumeClaim
	StorageClasss          []*storagev1.StorageClass
	PodDisruptionBudgets   []*policyv1beta1.PodDisruptionBudget
	Jobs                   []*batchv1.Job
	CronJobs               []*batchv1beta1.CronJob
	ConfigMaps             []*corev1.ConfigMap
}

type AppResource struct {
	Name     string
	Resource ResourceTypes
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
	// debug: this is a hack method
	// todo: must understand why channel is full
	watch.DefaultChanSize = 1000

	trace := utiltrace.New("Trace Simulate")
	defer trace.LogIfLong(1 * time.Second)

	var err error
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
	trace.Step("Trace Simulate make valid pod done")

	// init simulator
	sim, err := NewSimulator(opts...)
	if err != nil {
		return nil, err
	}
	defer func() {
		sim.Close()
	}()
	trace.Step("Trace Simulate init done")

	var failedPods []UnscheduledPod
	// run cluster
	result, err := sim.RunCluster(cluster)
	if err != nil {
		return nil, err
	}
	failedPods = append(failedPods, result.UnscheduledPods...)
	trace.Step("Trace Simulate run cluster done")

	// schedule pods
	for _, app := range apps {
		result, err = sim.ScheduleApp(app)
		if err != nil {
			return nil, err
		}
		failedPods = append(failedPods, result.UnscheduledPods...)
	}
	result.UnscheduledPods = failedPods
	trace.Step("Trace Simulate schedule app done")

	return result, nil
}
