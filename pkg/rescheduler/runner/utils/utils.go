package utils

import (
	"github.com/alibaba/open-simulator/pkg/simulator"
	simontype "github.com/alibaba/open-simulator/pkg/type"
	"github.com/alibaba/open-simulator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	resourcehelper "k8s.io/kubectl/pkg/util/resource"
	kubetypes "k8s.io/kubernetes/pkg/kubelet/types"
)

type PodSlice []*corev1.Pod

func NormalizePodsNodes(nodes []corev1.Node, pods []corev1.Pod) ([]*corev1.Node, []*corev1.Pod, error) {
	normalizedNodes, normalizedPods := make([]*corev1.Node, 0), make([]*corev1.Pod, 0)
	for _, node := range nodes {
		nodeCopy := node.DeepCopy()
		newNode, err := utils.MakeValidNodeByNode(nodeCopy, nodeCopy.Name)
		if err != nil {
			return nil, nil, err
		}
		normalizedNodes = append(normalizedNodes, newNode)
	}
	for _, pod := range pods {
		podCopy := pod.DeepCopy()
		newPod, err := utils.MakeValidPod(podCopy)
		if err != nil {
			return nil, nil, err
		}
		normalizedPods = append(normalizedPods, newPod)
	}

	return normalizedNodes, normalizedPods, nil
}

func GetMastersAndWorkers(nodes []corev1.Node) ([]corev1.Node, []corev1.Node) {
	var masters, workers []corev1.Node
	for _, node := range nodes {
		if _, exist := node.Labels["node-role.kubernetes.io/master"]; exist {
			masters = append(masters, node)
		} else {
			workers = append(workers, node)
		}
	}

	return masters, workers
}

func BuildMapForNodesPods(nodes []*corev1.Node, pods []*corev1.Pod) map[*corev1.Node]PodSlice {
	layout := make(map[*corev1.Node]PodSlice)
	for _, node := range nodes {
		newNode := node.DeepCopy()
		layout[newNode] = make(PodSlice, 0)
	}

	for node := range layout {
		for _, pod := range pods {
			if pod.Spec.NodeName == node.Name {
				newPod := pod.DeepCopy()
				layout[node] = append(layout[node], newPod)
			}
		}
	}

	return layout
}

func DeepCopyLayoutForNodesPods(layout map[*corev1.Node]PodSlice) map[*corev1.Node]PodSlice {
	newLayout := make(map[*corev1.Node]PodSlice)

	for node, podSlice := range layout {
		newNode := node.DeepCopy()
		newLayout[newNode] = make(PodSlice, 0)
		for _, pod := range podSlice {
			newPod := pod.DeepCopy()
			newLayout[newNode] = append(newLayout[newNode], newPod)
		}
	}

	return newLayout
}

func GetPodsTotalRequestsExcludeStaticAndDaemonPod(pods []*corev1.Pod) map[corev1.ResourceName]resource.Quantity {
	reqs := map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceCPU:    *resource.NewQuantity(0, resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(0, resource.DecimalSI),
	}

	for _, pod := range pods {
		if OwnedByDaemonset(pod.OwnerReferences) || kubetypes.IsStaticPod(pod) || kubetypes.IsMirrorPod(pod) {
			continue
		}
		podReqs, _ := resourcehelper.PodRequestsAndLimits(pod)
		for podReqName, podReqValue := range podReqs {
			if value, ok := reqs[podReqName]; !ok {
				reqs[podReqName] = podReqValue.DeepCopy()
			} else {
				value.Add(podReqValue)
				reqs[podReqName] = value
			}
		}
	}

	return reqs
}

func TaintExists(taint corev1.Taint, taints []corev1.Taint) bool {
	for _, t := range taints {
		if t == taint {
			return true
		}
	}

	return false
}

func RemoveDaemonAndStaticPod(pods []*corev1.Pod) []*corev1.Pod {
	var newPods []*corev1.Pod
	for _, pod := range pods {
		if !OwnedByDaemonset(pod.OwnerReferences) && !kubetypes.IsMirrorPod(pod) && !kubetypes.IsStaticPod(pod) {
			newPods = append(newPods, pod)
		}
	}

	return newPods
}

func AddOriginatedFromWhichNodeAnnotation(pods []*corev1.Pod) {
	for _, pod := range pods {
		if _, exist := pod.Annotations["originated-from"]; !exist {
			pod.Annotations["originated-from"] = pod.Spec.NodeName
		}
	}
}

func InitNodeNameOfPodsOnNode(pods []*corev1.Pod) {
	for _, pod := range pods {
		pod.Spec.NodeName = ""
	}
}

func SetNoScheduleTaintOnNode(node *corev1.Node) {
	unschedulableTaint := corev1.Taint{
		Key:    corev1.TaintNodeUnschedulable,
		Effect: corev1.TaintEffectNoSchedule,
	}
	node.Spec.Taints = append(node.Spec.Taints, unschedulableTaint)
}

func GetClusterArgsForSimulation(layout map[*corev1.Node]PodSlice) simulator.ResourceTypes {
	var nodes []*corev1.Node
	var pods []*corev1.Pod
	for node, podsOnNode := range layout {
		nodes = append(nodes, node)
		pods = append(pods, podsOnNode...)
	}

	return simulator.ResourceTypes{
		Nodes: nodes,
		Pods:  pods,
	}
}

func UpdateLayoutByResult(nodeStatus []simulator.NodeStatus) map[*corev1.Node]PodSlice {
	newLayout := make(map[*corev1.Node]PodSlice)
	for _, status := range nodeStatus {
		newLayout[status.Node] = status.Pods
	}
	return newLayout
}

func OwnedByDaemonset(refs []metav1.OwnerReference) bool {
	for _, ref := range refs {
		if ref.Kind == simontype.DaemonSet {
			return true
		}
	}
	return false
}
