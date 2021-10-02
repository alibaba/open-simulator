package algo

import (
	corev1 "k8s.io/api/core/v1"
)

type AffinityQueue struct {
	pods []*corev1.Pod
}

func NewAffinityQueue(pods []*corev1.Pod) *AffinityQueue {
	return &AffinityQueue{
		pods: pods,
	}
}

func (aff *AffinityQueue) Len() int      { return len(aff.pods) }
func (aff *AffinityQueue) Swap(i, j int) { aff.pods[i], aff.pods[j] = aff.pods[j], aff.pods[i] }
func (aff *AffinityQueue) Less(i, j int) bool {
	if aff.pods[i].Spec.NodeSelector == nil {
		return true
	}
	return false
}
