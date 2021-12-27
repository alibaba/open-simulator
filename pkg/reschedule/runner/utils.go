package runner

import (
	simontype "github.com/alibaba/open-simulator/pkg/type"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	resourcehelper "k8s.io/kubectl/pkg/util/resource"
	kubetypes "k8s.io/kubernetes/pkg/kubelet/types"
)

func GetWorkersAndMasters(nodes []corev1.Node) ([]corev1.Node, []corev1.Node) {
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

type PodSlice []*corev1.Pod

func BuildMapForNodesPods(nodes []corev1.Node, pods []corev1.Pod) map[*corev1.Node]PodSlice {
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

func RemoveDaemonPod(pods []*corev1.Pod) []*corev1.Pod {
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

func OwnedByDaemonset(refs []metav1.OwnerReference) bool {
	for _, ref := range refs {
		if ref.Kind == simontype.DaemonSet {
			return true
		}
	}
	return false
}
