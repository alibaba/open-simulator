package utils

import "k8s.io/api/core/v1"

// IsGPUSharingNode Is the Node for GPU sharing
func IsGPUSharingNode(node *v1.Node) bool {
	return GetTotalGPUMemory(node) > 0
}

// GetTotalGPUMemory Get the total GPU memory of the Node
func GetTotalGPUMemory(node *v1.Node) int {
	val, ok := node.Status.Capacity[ResourceName]
	if !ok {
		return 0
	}
	return int(val.Value())
}

// GetGPUCountInNode Get the GPU count of the node
func GetGPUCountInNode(node *v1.Node) int {
	val, ok := node.Status.Capacity[CountName]
	if !ok {
		return 0
	}
	return int(val.Value())
}

func GetGPUModel(node *v1.Node) string {
	val, ok := node.ObjectMeta.Labels[ModelName]
	if !ok {
		return "N/A"
	}
	return val
}
