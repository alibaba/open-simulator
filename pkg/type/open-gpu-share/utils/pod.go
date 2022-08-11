package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// AssignedNonTerminatedPod selects pods that are assigned and non-terminal (scheduled and running).
func AssignedNonTerminatedPod(pod *v1.Pod) bool {
	if pod.DeletionTimestamp != nil {
		return false
	}

	if len(pod.Spec.NodeName) == 0 {
		return false
	}
	if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
		return false
	}
	return true
}

// IsCompletePod determines if the pod is complete
func IsCompletePod(pod *v1.Pod) bool {
	if pod.DeletionTimestamp != nil {
		return true
	}

	if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
		return true
	}
	return false
}

// GetGpuIdFromAnnotation gets GPU ID from Annotation, could be "1" or "0-1-2-3" for multi-GPU allocations
func GetGpuIdFromAnnotation(pod *v1.Pod) (id string) {
	if len(pod.ObjectMeta.Annotations) > 0 {
		if value, found := pod.ObjectMeta.Annotations[DeviceIndex]; found {
			id += value
		}
	}
	return id
}

// GetGpuIdListFromAnnotation gets GPU ID List from Annotation, could be [1] or [0, 1, 2, 3] for multi-GPU allocations
func GetGpuIdListFromAnnotation(pod *v1.Pod) (idl []int, err error) {
	id := GetGpuIdFromAnnotation(pod)
	return GpuIdStrToIntList(id)
}

// GetGpuMemoryFromPodAnnotation gets the GPU Memory of the pod
func GetGpuMemoryFromPodAnnotation(pod *v1.Pod) (gpuMemory int64) {
	if len(pod.ObjectMeta.Annotations) > 0 {
		if value, found := pod.ObjectMeta.Annotations[ResourceName]; found {
			if q, err := resource.ParseQuantity(value); err == nil {
				gpuMemory += q.Value()
			}
		}
	}
	//log.Printf("debug: pod %s in ns %s with status %v has GPU Mem %d", pod.Name, pod.Namespace, pod.Status.Phase, gpuMemory)
	return gpuMemory
}

// GetGpuCountFromPodAnnotation gets the GPU Count of the pod
func GetGpuCountFromPodAnnotation(pod *v1.Pod) (gpuCount int64) {
	if len(pod.ObjectMeta.Annotations) > 0 {
		if value, found := pod.ObjectMeta.Annotations[CountName]; found {
			if val, err := strconv.Atoi(value); err == nil && val >= 0 {
				gpuCount += int64(val)
			}
		}
	}
	//log.Printf("debug: pod %s in ns %s with status %v has GPU Count %d", pod.Name, pod.Namespace, pod.Status.Phase, gpuCount)
	return gpuCount
}

// GetGpuMemoryAndCountFromPodAnnotation gets GPU Memory (for each GPU) and GPU Number requested by the Pod
func GetGpuMemoryAndCountFromPodAnnotation(pod *v1.Pod) (gpuMem int64, gpuNum int64) {
	if len(pod.ObjectMeta.Annotations) > 0 {
		if value, found := pod.ObjectMeta.Annotations[ResourceName]; found {
			if q, err := resource.ParseQuantity(value); err == nil {
				gpuMem += q.Value()
			}
		}
		if value, found := pod.ObjectMeta.Annotations[CountName]; found {
			if val, err := strconv.Atoi(value); err == nil && val >= 0 {
				gpuNum += int64(val)
			}
		}
	}
	//log.Printf("debug: pod %s in ns %s with status %v has GPU Mem %d, GPU Count %d", pod.Name, pod.Namespace, pod.Status.Phase, gpuMem, gpuNum)
	return gpuMem, gpuNum
}

// GpuIdStrToIntList follows the string formed in func (n *GpuNodeInfo) AllocateGpuId
func GpuIdStrToIntList(id string) (idl []int, err error) {
	if len(id) <= 0 {
		return idl, nil
	}
	res := strings.Split(id, "-") // like "2-3-4" -> [2, 3, 4]
	for _, idxStr := range res {
		if idx, e := strconv.Atoi(idxStr); e == nil {
			idl = append(idl, idx)
		} else {
			return idl, e
		}
	}
	return idl, nil
}

// GetUpdatedPodAnnotationSpec updates pod env with devId
func GetUpdatedPodAnnotationSpec(oldPod *v1.Pod, devId string) (newPod *v1.Pod) {
	newPod = oldPod.DeepCopy()
	if len(newPod.ObjectMeta.Annotations) == 0 {
		newPod.ObjectMeta.Annotations = map[string]string{}
	}

	now := time.Now()
	newPod.ObjectMeta.Annotations[DeviceIndex] = devId
	newPod.ObjectMeta.Annotations[AssumeTime] = fmt.Sprintf("%d", now.UnixNano())
	return newPod
}
