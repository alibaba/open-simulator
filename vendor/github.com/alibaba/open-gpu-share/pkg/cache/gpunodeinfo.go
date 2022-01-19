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
	gpuTotalMemory int64
	model          string
	rwmu           *sync.RWMutex
}

// NewGpuNodeInfo creates Node Level
func NewGpuNodeInfo(node *v1.Node) *GpuNodeInfo {
	//log.Printf("debug: NewGpuNodeInfo() creates nodeInfo for %s", node.Name)

	cardModel := utils.GetGpuModel(node)
	devMap := map[int]*DeviceInfo{}
	for i := 0; i < utils.GetGpuCountInNode(node); i++ {
		devMap[i] = newDeviceInfo(i, utils.GetTotalGpuMemory(node)/int64(utils.GetGpuCountInNode(node)), cardModel)
	}

	//if len(devMap) == 0 {
	//	log.Printf("warn: node %s with nodeinfo %v has no devices", node.Name, node)
	//}

	return &GpuNodeInfo{
		name:           node.Name,
		node:           node,
		devs:           devMap,
		gpuCount:       utils.GetGpuCountInNode(node),
		gpuTotalMemory: utils.GetTotalGpuMemory(node),
		model:          cardModel,
		rwmu:           new(sync.RWMutex),
	}
}

// Reset only updates the devices when the length of devs is 0
func (n *GpuNodeInfo) Reset(node *v1.Node) {
	n.gpuCount = utils.GetGpuCountInNode(node)
	n.gpuTotalMemory = utils.GetTotalGpuMemory(node)
	n.node = node

	if len(n.devs) == 0 && n.gpuCount > 0 {
		cardModel := utils.GetGpuModel(node)
		devMap := map[int]*DeviceInfo{}
		for i := 0; i < utils.GetGpuCountInNode(node); i++ {
			devMap[i] = newDeviceInfo(i, n.gpuTotalMemory/int64(n.gpuCount), cardModel)
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

func (n *GpuNodeInfo) GetTotalGpuMemory() int64 {
	return n.gpuTotalMemory
}

func (n *GpuNodeInfo) GetGpuCount() int {
	return n.gpuCount
}

func (n *GpuNodeInfo) removePod(pod *v1.Pod) {
	n.rwmu.Lock()
	defer n.rwmu.Unlock()

	if idl, err := utils.GetGpuIdListFromAnnotation(pod); err == nil {
		for _, idx := range idl {
			if dev, found := n.devs[(idx)]; found {
				dev.removePod(pod)
			} else {
				log.Printf("warn: Pod %s in ns %s failed to find the GPU ID %d in node %s", pod.Name, pod.Namespace, idx, n.name)
			}
		}
	} else {
		log.Printf("warn: Pod %s in ns %s has problem with parsing GPU ID %d in node %s, error: %s", pod.Name, pod.Namespace, idl, n.name, err)
	}
}

// Add the Pod which has the GPU id to the node
func (n *GpuNodeInfo) addOrUpdatePod(pod *v1.Pod) (added bool) {
	n.rwmu.Lock()
	defer n.rwmu.Unlock()

	added = false
	if idl, err := utils.GetGpuIdListFromAnnotation(pod); err == nil {
		for _, idx := range idl {
			if dev, found := n.devs[(idx)]; found {
				dev.addPod(pod)
				added = true
			} else {
				log.Printf("warn: Pod %s in ns %s failed to find the GPU ID %d in node %s", pod.Name, pod.Namespace, idx, n.name)
			}
		}
	} else {
		log.Printf("warn: Pod %s in ns %s has problem with parsing GPU ID %d in node %s, error: %s", pod.Name, pod.Namespace, idl, n.name, err)
	}
	return added
}

// Assume checks if the pod can be allocated on the node
func (n *GpuNodeInfo) Assume(pod *v1.Pod) (allocatable bool) {
	allocatable = false

	n.rwmu.RLock()
	defer n.rwmu.RUnlock()

	availableGpus := n.getAvailableGpus()
	reqGpuMem := int64(utils.GetGpuMemoryFromPodResource(pod))
	//log.Printf("debug: AvailableGPUs: %v in node %s", availableGpus, n.name)

	if len(availableGpus) > 0 {
		for devId := 0; devId < len(n.devs); devId++ {
			availableGpu, ok := availableGpus[devId]
			if ok {
				if availableGpu >= reqGpuMem {
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
	devId, found := n.AllocateGpuId(pod)
	if found {
		//log.Printf("info: Allocate() 1. Allocate GPU ID %d to pod %s in ns %s.----", devId, pod.Name, pod.Namespace)
		patchedAnnotationBytes, err := utils.PatchPodAnnotationSpec(pod, devId, n.GetTotalGpuMemory()/int64(n.GetGpuCount()))
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
		if idl, e := utils.GpuIdStrToIntList(devId); e == nil {
			for _, idx := range idl {
				if dev, found := n.devs[(idx)]; found {
					dev.addPod(newPod)
				} else {
					log.Printf("warn: Pod %s in ns %s failed to find the GPU ID %d in node %s", pod.Name, pod.Namespace, idx, n.name)
				}
			}
		} else {
			log.Printf("warn: Pod %s in ns %s has problem with parsing GPU ID %d in node %s, error: %s", pod.Name, pod.Namespace, idl, n.name, e)
		}
	}
	//log.Printf("info: Allocate() ----End to allocate GPU for gpu mem for pod %s in ns %s----", pod.Name, pod.Namespace)
	return err
}

// AllocateGpuId is the key of GPU allocating; it assigns the GPU ID to the pod
func (n *GpuNodeInfo) AllocateGpuId(pod *v1.Pod) (candDevId string, found bool) {
	found = false
	candDevId = ""
	// Assuming one Pod has only 1 GPU container; if not so, we let the containers share the same GPU memory spec, i.e.,
	// Resources.Limits[ResourceName] is the GPU mem spec for each GPU for all containers.
	reqGpuMem, reqGpuNum := utils.GetGpuMemoryAndCountFromPodResource(pod) // reqGpuMem * reqGpuNum = totalGpuMemReq
	if reqGpuMem <= 0 || reqGpuNum <= 0 {
		return candDevId, found
	}

	availableGpus := n.getAvailableGpus()
	if len(availableGpus) <= 0 {
		return candDevId, found
	}

	if id := utils.GetGpuIdFromAnnotation(pod); len(id) > 0 {
		if idl, err := utils.GpuIdStrToIntList(id); err == nil && len(idl) > 0 { // just to validate id; not return idl.
			return id, true
		} else {
			log.Printf("warn: pod (%s) %s has invalid GPU ID in Annotation %s: %s", pod.Namespace, pod.Name, utils.EnvResourceIndex, id)
		}
	}

	if reqGpuNum == 1 { // 1-GPU pod. Adopt the original naive packing logic
		var candGpuMem int64
		for devId := 0; devId < len(n.devs); devId++ {
			if idleGpuMem, ok := availableGpus[devId]; ok {
				if idleGpuMem >= reqGpuMem {
					if candDevId == "" || idleGpuMem < candGpuMem {
						candDevId = strconv.Itoa(devId)
						candGpuMem = idleGpuMem // update to the tightest fit
						found = true
					}
				}
			}
		}
	} else { // multi-GPU pod. Greedy algorithm. Trying to pack as many containers onto 1 GPU as possible.
		var candDevIdList []int
		devId, reqGpuId := 0, 0
		for devId < len(n.devs) && reqGpuId < int(reqGpuNum) { // two pointers
			if idleGpuMem, ok := availableGpus[devId]; ok && idleGpuMem >= reqGpuMem {
				candDevIdList = append(candDevIdList, devId)
				availableGpus[devId] = idleGpuMem - reqGpuMem
				reqGpuId++
			} else {
				devId++
			}
		}
		if reqGpuId == int(reqGpuNum) {
			candDevId = strconv.Itoa(candDevIdList[0])
			for _, id := range candDevIdList[1:] {
				candDevId += fmt.Sprintf("-%d", id)
			}
			found = true
		}
	}

	return candDevId, found
}

func (n *GpuNodeInfo) getAvailableGpus() (availableGpus map[int]int64) {
	allGpus := n.getAllGpus()
	usedGpus := n.getUsedGpus()
	//unhealthyGpus := n.getUnhealthyGpus()
	availableGpus = map[int]int64{}
	for id, totalGpuMem := range allGpus {
		if usedGpuMem, found := usedGpus[id]; found {
			availableGpus[id] = totalGpuMem - usedGpuMem
		}
	}
	//log.Printf("info: available GPU list %v before removing unhealty GPUs", availableGpus)
	//for id, _ := range unhealthyGpus {
	//	log.Printf("info: delete dev %d from availble GPU list", id)
	//	delete(availableGpus, id)
	//}
	//log.Printf("info: available GPU list %v after removing unhealty GPUs", availableGpus)

	return availableGpus
}

// device index: gpu memory
func (n *GpuNodeInfo) getUsedGpus() (usedGpus map[int]int64) {
	usedGpus = map[int]int64{}
	for _, dev := range n.devs {
		usedGpus[dev.idx] = dev.GetUsedGpuMemory()
	}
	//log.Printf("info: getUsedGpus: %v in node %s, and devs %v", usedGpus, n.name, n.devs)
	return usedGpus
}

// device index: gpu memory
func (n *GpuNodeInfo) getAllGpus() (allGpus map[int]int64) {
	allGpus = map[int]int64{}
	for _, dev := range n.devs {
		allGpus[dev.idx] = dev.totalGpuMem
	}
	//log.Printf("info: getAllGpus: %v in node %s, and dev %v", allGpus, n.name, n.devs)
	return allGpus
}

// getUnhealthyGpus get the unhealthy GPUs from configmap
func (n *GpuNodeInfo) getUnhealthyGpus() (unhealthyGpus map[int]bool) {
	unhealthyGpus = map[int]bool{}
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
			unhealthyGpus[id] = true
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
//			utils.EnvResourceByDev:      fmt.Sprintf("%d", totalGpuMemByDev),
//			utils.EnvResourceByPod:      fmt.Sprintf("%d", GetGpuMemoryFromPodResource(oldPod)),
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
