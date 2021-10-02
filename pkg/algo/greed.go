package algo

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	resourcehelper "k8s.io/kubectl/pkg/util/resource"
)

type GreedQueue struct {
	pods          []*corev1.Pod
	totalResource corev1.ResourceList
}

func NewGreedQueue(nodes []corev1.Node, pods []*corev1.Pod) *GreedQueue {
	totalResource := map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceCPU:    *resource.NewQuantity(0, resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(0, resource.DecimalSI),
	}
	for _, node := range nodes {
		cpu := totalResource[corev1.ResourceCPU]
		memory := totalResource[corev1.ResourceMemory]
		cpu.Add(*node.Status.Allocatable.Cpu())
		memory.Add(*node.Status.Allocatable.Memory())
		totalResource[corev1.ResourceCPU] = cpu
		totalResource[corev1.ResourceMemory] = memory
	}
	return &GreedQueue{
		totalResource: totalResource,
		pods:          pods,
	}
}

func (greed *GreedQueue) Len() int      { return len(greed.pods) }
func (greed *GreedQueue) Swap(i, j int) { greed.pods[i], greed.pods[j] = greed.pods[j], greed.pods[i] }
func (greed *GreedQueue) Less(i, j int) bool {
	// DRF算法: 在调度时，让具有最低资源占用比例的任务具有高优先级
	// 若用DRF算法, 反而不是最优解
	if greed.calculatePodShare(greed.pods[i]) > greed.calculatePodShare(greed.pods[j]) {
		return true
	}
	return false
}

func (greed *GreedQueue) calculatePodShare(pod *corev1.Pod) float64 {
	podReq, _ := resourcehelper.PodRequestsAndLimits(pod)
	if len(podReq) == 0 {
		return 0
	}

	res := float64(0)
	for resourceName := range greed.totalResource {
		allocatedRes := podReq[resourceName]
		totalRes := greed.totalResource[resourceName]
		share := Share(allocatedRes.AsApproximateFloat64(), totalRes.AsApproximateFloat64())
		if share > res {
			res = share
		}
	}

	return res
}

// Share is used to determine the share
func Share(alloc, total float64) float64 {
	var share float64
	if total == 0 {
		if alloc == 0 {
			share = 0
		} else {
			share = 1
		}
	} else {
		share = alloc / total
	}

	return share
}
