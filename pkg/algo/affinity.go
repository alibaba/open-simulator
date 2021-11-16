package algo

import (
	corev1 "k8s.io/api/core/v1"
)

// AffinityQueue is used to sort pods by Affinity
type AffinityQueue struct {
	pods []*corev1.Pod
}

// NewGreedQueue return a AffinityQueue
func NewAffinityQueue(pods []*corev1.Pod) *AffinityQueue {
	return &AffinityQueue{
		pods: pods,
	}
}

func (aff *AffinityQueue) Len() int      { return len(aff.pods) }
func (aff *AffinityQueue) Swap(i, j int) { aff.pods[i], aff.pods[j] = aff.pods[j], aff.pods[i] }
func (aff *AffinityQueue) Less(i, j int) bool {
	return aff.pods[i].Spec.NodeSelector != nil
}
