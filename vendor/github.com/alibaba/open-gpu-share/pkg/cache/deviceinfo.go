package cache

import (
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"
	"log"
	"sync"

	"github.com/alibaba/open-gpu-share/pkg/utils"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type DeviceInfo struct {
	idx         int
	podMap      map[types.UID]*v1.Pod
	model       string
	totalGPUMem uint
	rwmu        *sync.RWMutex
}

func (d *DeviceInfo) GetPods() []*v1.Pod {
	pods := []*v1.Pod{}
	for _, pod := range d.podMap {
		pods = append(pods, pod)
	}
	return pods
}

func newDeviceInfo(index int, totalGPUMem uint, cardModel string) *DeviceInfo {
	return &DeviceInfo{
		idx:         index,
		totalGPUMem: totalGPUMem,
		podMap:      map[types.UID]*v1.Pod{},
		model:       cardModel,
		rwmu:        new(sync.RWMutex),
	}
}

func (d *DeviceInfo) GetTotalGPUMemory() uint {
	return d.totalGPUMem
}

func (d *DeviceInfo) GetUsedGPUMemory() (gpuMem uint) {
	//log.Printf("debug: GetUsedGPUMemory() podMap %v, and its address is %p", d.podMap, d)
	d.rwmu.RLock()
	defer d.rwmu.RUnlock()
	for _, pod := range d.podMap {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			log.Printf("debug: skip the pod %s in ns %s due to its status is %s", pod.Name, pod.Namespace, pod.Status.Phase)
			continue
		}
		// gpuMem += utils.GetGPUMemoryFromPodEnv(pod)
		gpuMem += utils.GetGPUMemoryFromPodAnnotation(pod)
	}
	return gpuMem
}

func (d *DeviceInfo) addPod(pod *v1.Pod) {
	//log.Printf("debug: dev.addPod() Pod %s in ns %s with the GPU ID %d will be added to device map", pod.Name, pod.Namespace, d.idx)
	d.rwmu.Lock()
	defer d.rwmu.Unlock()
	d.podMap[pod.UID] = pod
	//log.Printf("debug: dev.addPod() after updated is %v, and its address is %p", d.podMap, d)
}

func (d *DeviceInfo) removePod(pod *v1.Pod) {
	//log.Printf("debug: dev.removePod() Pod %s in ns %s with the GPU ID %d will be removed from device map", pod.Name, pod.Namespace, d.idx)
	d.rwmu.Lock()
	defer d.rwmu.Unlock()
	delete(d.podMap, pod.UID)
	//log.Printf("debug: dev.removePod() after updated is %v, and its address is %p", d.podMap, d)
}

type DeviceInfoBrief struct {
	idx            int
	model          string
	PodList        []string
	GpuTotalMemory resource.Quantity
	GpuUsedMemory  resource.Quantity
}

func (d *DeviceInfo) ExportDeviceInfoBrief() *DeviceInfoBrief {
	var podList []string
	for _, pod := range d.podMap {
		podList = append(podList, fmt.Sprintf("%s:%s", pod.Namespace, pod.Name))
	}
	gpuUsedMem, _ := resource.ParseQuantity(fmt.Sprintf("%dMi", d.GetUsedGPUMemory()/(1024*1024)))
	gpuTotalMem, _ := resource.ParseQuantity(fmt.Sprintf("%dMi", d.totalGPUMem/(1024*1024)))
	return &DeviceInfoBrief{d.idx, d.model, podList, gpuTotalMem, gpuUsedMem}
}
