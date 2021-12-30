package cache

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/alibaba/open-gpu-share/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const (
	OptimisticLockErrorMsg = "the object has been modified; please apply your changes to the latest version and try again"
)

// GpuNodeInfo is node level aggregated information.
type GpuNodeInfo struct {
	name           string
	node           *v1.Node
	devs           map[int]*DeviceInfo
	gpuCount       int
	gpuTotalMemory int
	model          string
	rwmu           *sync.RWMutex
}

// Create Node Level
func NewGpuNodeInfo(node *v1.Node) *GpuNodeInfo {
	//log.Printf("debug: NewGpuNodeInfo() creates nodeInfo for %s", node.Name)

	cardModel := utils.GetGPUModel(node)
	devMap := map[int]*DeviceInfo{}
	for i := 0; i < utils.GetGPUCountInNode(node); i++ {
		devMap[i] = newDeviceInfo(i, uint(utils.GetTotalGPUMemory(node)/utils.GetGPUCountInNode(node)), cardModel)
	}

	//if len(devMap) == 0 {
	//	log.Printf("warn: node %s with nodeinfo %v has no devices", node.Name, node)
	//}

	return &GpuNodeInfo{
		name:           node.Name,
		node:           node,
		devs:           devMap,
		gpuCount:       utils.GetGPUCountInNode(node),
		gpuTotalMemory: utils.GetTotalGPUMemory(node),
		model:          cardModel,
		rwmu:           new(sync.RWMutex),
	}
}

// Only update the devices when the length of devs is 0
func (n *GpuNodeInfo) Reset(node *v1.Node) {
	n.gpuCount = utils.GetGPUCountInNode(node)
	n.gpuTotalMemory = utils.GetTotalGPUMemory(node)
	n.node = node
	//if n.gpuCount == 0 {
	//	log.Printf("warn: Reset for node %s but the gpu count is 0", node.Name)
	//}

	//if n.gpuTotalMemory == 0 {
	//	log.Printf("warn: Reset for node %s but the gpu total memory is 0", node.Name)
	//}

	if len(n.devs) == 0 && n.gpuCount > 0 {
		cardModel := utils.GetGPUModel(node)
		devMap := map[int]*DeviceInfo{}
		for i := 0; i < utils.GetGPUCountInNode(node); i++ {
			devMap[i] = newDeviceInfo(i, uint(n.gpuTotalMemory/n.gpuCount), cardModel)
		}
		n.devs = devMap
	}
	//log.Printf("info: Reset() update nodeInfo for %s with devs %v", node.Name, n.devs)
}

func (n *GpuNodeInfo) GetName() string {
	return n.name
}

func (n *GpuNodeInfo) GetDevs() []*DeviceInfo {
	devs := make([]*DeviceInfo, n.gpuCount)
	for i, dev := range n.devs {
		devs[i] = dev
	}
	return devs
}

func (n *GpuNodeInfo) GetDevByDevId(devId int) (*DeviceInfo, bool) {
	dev, found := n.devs[devId]
	return dev, found
}

func (n *GpuNodeInfo) GetNode() *v1.Node {
	return n.node
}

func (n *GpuNodeInfo) GetTotalGPUMemory() int {
	return n.gpuTotalMemory
}

func (n *GpuNodeInfo) GetGPUCount() int {
	return n.gpuCount
}

func (n *GpuNodeInfo) removePod(pod *v1.Pod) {
	n.rwmu.Lock()
	defer n.rwmu.Unlock()

	id := utils.GetGPUIDFromAnnotation(pod)
	if id >= 0 {
		dev, found := n.devs[id]
		if !found {
			//log.Printf("warn: Pod %s in ns %s failed to find the GPU ID %d in node %s", pod.Name, pod.Namespace, id, n.name)
		} else {
			dev.removePod(pod)
		}
	} else {
		//log.Printf("warn: Pod %s in ns %s is not set the GPU ID %d in node %s", pod.Name, pod.Namespace, id, n.name)
	}
}

// Add the Pod which has the GPU id to the node
func (n *GpuNodeInfo) addOrUpdatePod(pod *v1.Pod) (added bool) {
	n.rwmu.Lock()
	defer n.rwmu.Unlock()

	id := utils.GetGPUIDFromAnnotation(pod)
	//log.Printf("debug: addOrUpdatePod() Pod %s in ns %s with the GPU ID %d should be added to device map", pod.Name, pod.Namespace, id)
	if id >= 0 {
		dev, found := n.devs[id]
		if !found {
			//log.Printf("warn: Pod %s in ns %s failed to find the GPU ID %d in node %s", pod.Name, pod.Namespace, id, n.name)
		} else {
			dev.addPod(pod)
			added = true
		}
	} else {
		//log.Printf("warn: Pod %s in ns %s is not set the GPU ID %d in node %s", pod.Name, pod.Namespace, id, n.name)
	}
	return added
}

// Assume checks if the pod can be allocated on the node
func (n *GpuNodeInfo) Assume(pod *v1.Pod) (allocatable bool) {
	allocatable = false

	n.rwmu.RLock()
	defer n.rwmu.RUnlock()

	availableGPUs := n.getAvailableGPUs()
	reqGPU := uint(utils.GetGPUMemoryFromPodResource(pod))
	//log.Printf("debug: AvailableGPUs: %v in node %s", availableGPUs, n.name)

	if len(availableGPUs) > 0 {
		for devID := 0; devID < len(n.devs); devID++ {
			availableGPU, ok := availableGPUs[devID]
			if ok {
				if availableGPU >= reqGPU {
					allocatable = true
					break
				}
			}
		}
	}

	return allocatable

}

func (n *GpuNodeInfo) Allocate(clientset *kubernetes.Clientset, pod *v1.Pod) (err error) {
	var newPod *v1.Pod
	n.rwmu.Lock()
	defer n.rwmu.Unlock()
	//log.Printf("info: Allocate() ----Begin to allocate GPU for gpu mem for pod %s in ns %s----", pod.Name, pod.Namespace)
	// 1. Update the pod spec
	devId, found := n.AllocateGPUID(pod)
	if found {
		//log.Printf("info: Allocate() 1. Allocate GPU ID %d to pod %s in ns %s.----", devId, pod.Name, pod.Namespace)
		patchedAnnotationBytes, err := utils.PatchPodAnnotationSpec(pod, devId, n.GetTotalGPUMemory()/n.GetGPUCount())
		if err != nil {
			return fmt.Errorf("failed to generate patched annotations,reason: %v", err)
		}
		newPod, err = clientset.CoreV1().Pods(pod.Namespace).Patch(context.TODO(), pod.Name, types.StrategicMergePatchType, patchedAnnotationBytes, metav1.PatchOptions{})
		if err != nil {
			// the object has been modified; please apply your changes to the latest version and try again
			if err.Error() == OptimisticLockErrorMsg {
				// retry
				pod, err = clientset.CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				newPod, err = clientset.CoreV1().Pods(pod.Namespace).Patch(context.TODO(), pod.Name, types.StrategicMergePatchType, patchedAnnotationBytes, metav1.PatchOptions{})
				if err != nil {
					return err
				}
			} else {
				//log.Printf("failed to patch pod %v", pod)
				return err
			}
		}
	} else {
		err = fmt.Errorf("The node %s can't place the pod %s in ns %s,and the pod spec is %v", pod.Spec.NodeName, pod.Name, pod.Namespace, pod)
	}

	// 2. Bind the pod to the node
	if err == nil {
		binding := &v1.Binding{
			ObjectMeta: metav1.ObjectMeta{Name: pod.Name, UID: pod.UID},
			Target:     v1.ObjectReference{Kind: "Node", Name: n.name},
		}
		//log.Printf("info: Allocate() 2. Try to bind pod %s in %s namespace to node %s with %v", pod.Name, pod.Namespace, pod.Spec.NodeName, binding)
		err = clientset.CoreV1().Pods(pod.Namespace).Bind(context.TODO(), binding, metav1.CreateOptions{})
		if err != nil {
			//log.Printf("warn: Failed to bind the pod %s in ns %s due to %v", pod.Name, pod.Namespace, err)
			return err
		}
	}

	// 3. update the device info if the pod is update successfully
	if err == nil {
		//log.Printf("info: Allocate() 3. Try to add pod %s in ns %s to dev %d", pod.Name, pod.Namespace, devId)
		dev, found := n.devs[devId]
		if !found {
			//log.Printf("warn: Pod %s in ns %s failed to find the GPU ID %d in node %s", pod.Name, pod.Namespace, devId, n.name)
		} else {
			dev.addPod(newPod)
		}
	}
	//log.Printf("info: Allocate() ----End to allocate GPU for gpu mem for pod %s in ns %s----", pod.Name, pod.Namespace)
	return err
}

// allocate the GPU ID to the pod
func (n *GpuNodeInfo) AllocateGPUID(pod *v1.Pod) (candidateDevID int, found bool) {

	reqGPU := uint(0)
	found = false
	candidateDevID = -1
	candidateGPUMemory := uint(0)
	availableGPUs := n.getAvailableGPUs()

	reqGPU = uint(utils.GetGPUMemoryFromPodResource(pod))

	if reqGPU > uint(0) {
		//log.Printf("info: reqGPU for pod %s in ns %s: %d", pod.Name, pod.Namespace, reqGPU)
		//log.Printf("info: AvailableGPUs: %v in node %s", availableGPUs, n.name)
		if len(availableGPUs) > 0 {
			for devID := 0; devID < len(n.devs); devID++ {
				availableGPU, ok := availableGPUs[devID]
				if ok {
					if availableGPU >= reqGPU {
						if candidateDevID == -1 || candidateGPUMemory > availableGPU {
							candidateDevID = devID
							candidateGPUMemory = availableGPU
						}

						found = true
					}
				}
			}
		}

		if found {
			//log.Printf("info: Find candidate dev id %d for pod %s in ns %s successfully.", candidateDevID, pod.Name, pod.Namespace)
		} else {
			//log.Printf("warn: Failed to find available GPUs %d for the pod %s in the namespace %s", reqGPU, pod.Name, pod.Namespace)
			return -1, false
		}
	}

	return candidateDevID, found
}

func (n *GpuNodeInfo) getAvailableGPUs() (availableGPUs map[int]uint) {
	allGPUs := n.getAllGPUs()
	usedGPUs := n.getUsedGPUs()
	//unhealthyGPUs := n.getUnhealthyGPUs()
	availableGPUs = map[int]uint{}
	for id, totalGPUMem := range allGPUs {
		if usedGPUMem, found := usedGPUs[id]; found {
			availableGPUs[id] = totalGPUMem - usedGPUMem
		}
	}
	//log.Printf("info: available GPU list %v before removing unhealty GPUs", availableGPUs)
	//for id, _ := range unhealthyGPUs {
	//	log.Printf("info: delete dev %d from availble GPU list", id)
	//	delete(availableGPUs, id)
	//}
	//log.Printf("info: available GPU list %v after removing unhealty GPUs", availableGPUs)

	return availableGPUs
}

// device index: gpu memory
func (n *GpuNodeInfo) getUsedGPUs() (usedGPUs map[int]uint) {
	usedGPUs = map[int]uint{}
	for _, dev := range n.devs {
		usedGPUs[dev.idx] = dev.GetUsedGPUMemory()
	}
	//log.Printf("info: getUsedGPUs: %v in node %s, and devs %v", usedGPUs, n.name, n.devs)
	return usedGPUs
}

// device index: gpu memory
func (n *GpuNodeInfo) getAllGPUs() (allGPUs map[int]uint) {
	allGPUs = map[int]uint{}
	for _, dev := range n.devs {
		allGPUs[dev.idx] = dev.totalGPUMem
	}
	//log.Printf("info: getAllGPUs: %v in node %s, and dev %v", allGPUs, n.name, n.devs)
	return allGPUs
}

// getUnhealthyGPUs get the unhealthy GPUs from configmap
func (n *GpuNodeInfo) getUnhealthyGPUs() (unhealthyGPUs map[int]bool) {
	unhealthyGPUs = map[int]bool{}
	name := fmt.Sprintf("unhealthy-gpu-%s", n.GetName())
	//log.Printf("info: try to find unhealthy node %s", name)
	cm := getConfigMap(name)
	if cm == nil {
		return
	}

	if devicesStr, found := cm.Data["gpus"]; found {
		//log.Printf("warn: the unhelathy gpus %s", devicesStr)
		idsStr := strings.Split(devicesStr, ",")
		for _, sid := range idsStr {
			id, err := strconv.Atoi(sid)
			if err != nil {
				log.Printf("warn: failed to parse id %s due to %v", sid, err)
			}
			unhealthyGPUs[id] = true
		}
	} else {
		//log.Println("info: skip, because there are no unhealthy gpus")
	}

	return

}

//func (n *GpuNodeInfo) PatchNodeAnnotationSpec() ([]byte, error) {
//	now := time.Now()
//	patchAnnotations := map[string]interface{}{
//		"metadata": map[string]map[string]string{"annotations": {
//			utils.EnvResourceIndex:      fmt.Sprintf("%d", devId),
//			utils.EnvResourceByDev:      fmt.Sprintf("%d", totalGPUMemByDev),
//			utils.EnvResourceByPod:      fmt.Sprintf("%d", GetGPUMemoryFromPodResource(oldPod)),
//			utils.EnvAssignedFlag:       "false",
//			utils.EnvResourceAssumeTime: fmt.Sprintf("%d", now.UnixNano()),
//		}}}
//	return json.Marshal(patchAnnotations)
//}

type NodeGpuInfo struct {
	DevsBrief      map[int]*DeviceInfoBrief
	GpuCount       int
	GpuModel       string
	GpuTotalMemory resource.Quantity
	NumPods        int
}

func (n *GpuNodeInfo) ExportGpuNodeInfoAsNodeGpuInfo() *NodeGpuInfo {
	var numPods int
	devsBrief := map[int]*DeviceInfoBrief{}
	for idx, d := range n.devs {
		dib := d.ExportDeviceInfoBrief()
		devsBrief[idx] = dib
		numPods += len(dib.PodList)
	}
	gpuTotalMem, _ := resource.ParseQuantity(fmt.Sprintf("%dMi", n.gpuTotalMemory/(1024*1024)))
	return &NodeGpuInfo{devsBrief, n.gpuCount, n.model, gpuTotalMem, numPods}
}
