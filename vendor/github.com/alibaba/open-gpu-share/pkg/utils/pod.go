package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
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

// IsGpuSharingPod determines if it's the pod for GPU sharing
func IsGpuSharingPod(pod *v1.Pod) bool {
	return GetGpuMemoryFromPodResource(pod) > 0
}

// GetGpuIdFromAnnotation gets GPU ID from Annotation, could be "1" or "0-1-2-3" for multi-GPU allocations
func GetGpuIdFromAnnotation(pod *v1.Pod) string {
	id := ""
	if len(pod.ObjectMeta.Annotations) > 0 {
		value, found := pod.ObjectMeta.Annotations[EnvResourceIndex]
		if found {
			return value
		}
	}
	return id
}

// GetGpuIdListFromAnnotation gets GPU ID List from Annotation, could be [1] or [0, 1, 2, 3] for multi-GPU allocations
func GetGpuIdListFromAnnotation(pod *v1.Pod) (idl []int, err error) {
	id := GetGpuIdFromAnnotation(pod)
	return GpuIdStrToIntList(id)
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

// GetGpuIdFromEnv gets GPU ID from Env
func GetGpuIdFromEnv(pod *v1.Pod) int {
	id := -1
	for _, container := range pod.Spec.Containers {
		id = getGpuIdFromContainer(container)
		if id >= 0 {
			return id
		}
	}
	return id
}

func getGpuIdFromContainer(container v1.Container) (devIdx int) {
	devIdx = -1
	var err error
loop:
	for _, env := range container.Env {
		if env.Name == EnvResourceIndex {
			devIdx, err = strconv.Atoi(env.Value)
			if err != nil {
				log.Printf("warn: Failed due to %v for %s", err, container.Name)
				devIdx = -1
			}
			break loop
		}
	}

	return devIdx
}

// GetGpuMemoryFromPodAnnotation gets the GPU Memory of the pod, choose the larger one between gpu memory and gpu init container memory
func GetGpuMemoryFromPodAnnotation(pod *v1.Pod) (gpuMemory int64) {
	if len(pod.ObjectMeta.Annotations) > 0 {
		value, found := pod.ObjectMeta.Annotations[EnvResourceByPod]
		if found {
			s, _ := strconv.Atoi(value)
			if s < 0 {
				s = 0
			}
			gpuMemory += int64(s)
		}
	}
	//log.Printf("debug: pod %s in ns %s with status %v has GPU Mem %d", pod.Name, pod.Namespace, pod.Status.Phase, gpuMemory)
	return gpuMemory
}

// GetGpuMemoryFromPodEnv gets the GPU Memory of the pod, choose the larger one between gpu memory and gpu init container memory
func GetGpuMemoryFromPodEnv(pod *v1.Pod) (gpuMemory int64) {
	for _, container := range pod.Spec.Containers {
		gpuMemory += getGpuMemoryFromContainerEnv(container)
	}
	//log.Printf("debug: pod %s in ns %s with status %v has GPU Mem %d", pod.Name, pod.Namespace, pod.Status.Phase, gpuMemory)
	return gpuMemory
}

func getGpuMemoryFromContainerEnv(container v1.Container) (gpuMemory int64) {
	gpuMemory = 0
loop:
	for _, env := range container.Env {
		if env.Name == EnvResourceByPod {
			s, _ := strconv.Atoi(env.Value)
			if s < 0 {
				s = 0
			}
			gpuMemory = int64(s)
			break loop
		}
	}

	return gpuMemory
}

// GetGpuMemoryFromPodResource gets GPU Memory of the Pod
func GetGpuMemoryFromPodResource(pod *v1.Pod) int64 {
	total := int64(0)
	containers := pod.Spec.Containers
	for _, container := range containers {
		if val, ok := container.Resources.Limits[ResourceName]; ok {
			total += val.Value()
		}
	}
	return total
}

// GetGpuMemoryAndCountFromPodResource gets GPU Memory (for each GPU) and GPU Number requested by the Pod
func GetGpuMemoryAndCountFromPodResource(pod *v1.Pod) (int64, int64) {
	gpuMem, gpuNum := int64(0), int64(0)
	containers := pod.Spec.Containers
	for _, container := range containers {
		if val, ok := container.Resources.Limits[ResourceName]; ok {
			gpuMem += val.Value()
		}
		if val, ok := container.Resources.Limits[CountName]; ok {
			gpuNum += val.Value()
		}
	}
	if gpuMem > 0 && gpuNum <= 0 {
		log.Printf("warn: pod (%s) %s: GPU Mem = %d MiB, GPU Num = %d =(revised)=> 1", pod.Namespace, pod.Name, gpuMem/1024/1024, gpuNum)
		return gpuMem, 1
	}
	return gpuMem, gpuNum
}

// GetGpuMemoryFromPodResource gets GPU Memory of the Container
func GetGpuMemoryFromContainerResource(container v1.Container) int64 {
	total := int64(0)
	if val, ok := container.Resources.Limits[ResourceName]; ok {
		total += val.Value()
	}
	return total
}

// GetUpdatedPodEnvSpec updates pod env with devId
func GetUpdatedPodEnvSpec(oldPod *v1.Pod, devId string, totalGpuMemByDev int) (newPod *v1.Pod) {
	newPod = oldPod.DeepCopy()
	for i, c := range newPod.Spec.Containers {
		gpuMem := GetGpuMemoryFromContainerResource(c)

		if gpuMem > 0 {
			envs := []v1.EnvVar{
				// v1.EnvVar{Name: EnvNvGpu, Value: devId},
				v1.EnvVar{Name: EnvResourceIndex, Value: devId},
				v1.EnvVar{Name: EnvResourceByPod, Value: fmt.Sprintf("%d", gpuMem)},
				v1.EnvVar{Name: EnvResourceByDev, Value: fmt.Sprintf("%d", totalGpuMemByDev)},
				v1.EnvVar{Name: EnvAssignedFlag, Value: "false"},
			}

			for _, env := range envs {
				newPod.Spec.Containers[i].Env = append(newPod.Spec.Containers[i].Env,
					env)
			}
		}
	}

	return newPod
}

// GetUpdatedPodAnnotationSpec updates pod env with devId
func GetUpdatedPodAnnotationSpec(oldPod *v1.Pod, devId string, totalGpuMemByDev int64) (newPod *v1.Pod) {
	newPod = oldPod.DeepCopy()
	if len(newPod.ObjectMeta.Annotations) == 0 {
		newPod.ObjectMeta.Annotations = map[string]string{}
	}

	now := time.Now()
	newPod.ObjectMeta.Annotations[EnvResourceIndex] = devId
	newPod.ObjectMeta.Annotations[EnvResourceByDev] = fmt.Sprintf("%d", totalGpuMemByDev)
	newPod.ObjectMeta.Annotations[EnvResourceByPod] = fmt.Sprintf("%d", GetGpuMemoryFromPodResource(newPod))
	newPod.ObjectMeta.Annotations[EnvAssignedFlag] = "false"
	newPod.ObjectMeta.Annotations[EnvResourceAssumeTime] = fmt.Sprintf("%d", now.UnixNano())

	return newPod
}

func PatchPodAnnotationSpec(oldPod *v1.Pod, devId string, totalGpuMemByDev int64) ([]byte, error) {
	now := time.Now()
	patchAnnotations := map[string]interface{}{
		"metadata": map[string]map[string]string{"annotations": {
			EnvResourceIndex:      devId,
			EnvResourceByDev:      fmt.Sprintf("%d", totalGpuMemByDev),
			EnvResourceByPod:      fmt.Sprintf("%d", GetGpuMemoryFromPodResource(oldPod)),
			EnvAssignedFlag:       "false",
			EnvResourceAssumeTime: fmt.Sprintf("%d", now.UnixNano()),
		}}}
	return json.Marshal(patchAnnotations)
}
