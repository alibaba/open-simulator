package utils

import "k8s.io/api/core/v1"

// IsGpuSharingNode Is the Node for GPU sharing
func IsGpuSharingNode(node *v1.Node) bool {
	return GetTotalGpuMemory(node) > 0
}

// GetTotalGpuMemory Get the total GPU memory of the Node
func GetTotalGpuMemory(node *v1.Node) int64 {
	val, ok := node.Status.Capacity[ResourceName]
	if !ok {
		return 0
	}
	return val.Value()
}

// GetGpuCountInNode Get the GPU count of the node
func GetGpuCountInNode(node *v1.Node) int {
	val, ok := node.Status.Capacity[CountName]
	if !ok {
		return 0
	}
	return int(val.Value())
}

func GetGpuModel(node *v1.Node) string {
	val, ok := node.ObjectMeta.Labels[ModelName]
	if !ok {
		return "N/A"
	}
	return val
}
