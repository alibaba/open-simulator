package runner

import (
	"fmt"

	"github.com/alibaba/open-simulator/pkg/simulator"
	simontype "github.com/alibaba/open-simulator/pkg/type"
	"github.com/alibaba/open-simulator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DefaultRunner struct {
}

type DeschedulePlan struct {
	PodName      string
	PodNamespace string
	FromNode     string
	ToNode       string
	PodOwnerRefs []metav1.OwnerReference
}

type Runner interface {
	Run(allNodes []corev1.Node, allPods []corev1.Pod) ([]DeschedulePlan, error)
}

func NewDefaultRunner() Runner {
	return &DefaultRunner{}
}

func (runner DefaultRunner) Run(allNodes []corev1.Node, allPods []corev1.Pod) ([]DeschedulePlan, error) {
	// build parameters for Simulate function
	var newPods []*corev1.Pod
	for _, pod := range allPods {
		newPod := pod.DeepCopy()
		newPods = append(newPods, newPod)
	}
	var newNodes []*corev1.Node
	for _, node := range allNodes {
		newNode := node.DeepCopy()
		newNodes = append(newNodes, newNode)
	}

	// rstPods is used for saving result
	var rstPods map[string]corev1.Pod = make(map[string]corev1.Pod, len(allPods))
	for _, pod := range allPods {
		rstPods[fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)] = pod
	}

	for i := 1; i < len(allNodes); i++ {
		// step 1: select one node to be offline
		selectedNode := selectOneNode(newNodes, newPods)
		if selectedNode == nil {
			break
		}

		// step 2
		removeNodeNameOfPodsInSelectedNode(newPods, selectedNode.Name)

		// step 3: set taint for selected node to prevent other pods from being scheduled
		setNoScheduleTaintForNode(newNodes, selectedNode.Name)

		// step 4
		rst, err := simulator.Simulate(getClusterArgsForSimulation(newNodes, newPods), nil)
		if err != nil {
			return nil, err
		}

		if len(rst.UnscheduledPods) != 0 {
			break
		} else {
			fmt.Printf("remove node %s\n", selectedNode)
			rstPods = updateByResult(rst)
		}
	}

	var rstPlans []DeschedulePlan
	for _, pod := range allPods {
		fromNode := pod.Spec.NodeName
		toNode := rstPods[fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)].Spec.NodeName
		if fromNode != toNode {
			rstPlans = append(rstPlans, DeschedulePlan{
				PodName:      pod.Name,
				PodNamespace: pod.Namespace,
				FromNode:     fromNode,
				ToNode:       toNode,
				PodOwnerRefs: pod.OwnerReferences,
			})
		}
	}

	return rstPlans, nil
}

func selectOneNode(nodes []*corev1.Node, pods []*corev1.Pod) *corev1.Node {
	if len(nodes) == 1 {
		return nil
	}

	var tmpPod []corev1.Pod
	for _, pod := range pods {
		tmpPod = append(tmpPod, *pod)
	}

	var minScore int64 = 200
	var selectedNode *corev1.Node = nil
	for _, node := range nodes {
		// check if node is master
		if _, exist := node.Labels["node-role.kubernetes.io/master"]; exist {
			continue
		}
		// check if node contain Unschedulable taint
		exist := taintExists(corev1.Taint{
			Key:    corev1.TaintNodeUnschedulable,
			Effect: corev1.TaintEffectNoSchedule,
		}, node.Spec.Taints)
		if exist {
			continue
		}
		// get minimal score
		reqs, limits := utils.GetPodsTotalRequestsAndLimitsByNodeName(tmpPod, node.Name)
		nodeCpuReq, _, nodeMemoryReq, _, _, _ :=
			reqs[corev1.ResourceCPU], limits[corev1.ResourceCPU], reqs[corev1.ResourceMemory], limits[corev1.ResourceMemory], reqs[corev1.ResourceEphemeralStorage], limits[corev1.ResourceEphemeralStorage]
		allocatable := node.Status.Allocatable
		nodeFractionCpuReq := float64(nodeCpuReq.MilliValue()) / float64(allocatable.Cpu().MilliValue()) * 100
		nodeFractionMemoryReq := float64(nodeMemoryReq.Value()) / float64(allocatable.Memory().Value()) * 100
		if minScore > (int64(nodeFractionCpuReq) + int64(nodeFractionMemoryReq)) {
			selectedNode = node.DeepCopy()
			minScore = int64(nodeFractionCpuReq) + int64(nodeFractionMemoryReq)
		}
	}

	return selectedNode
}

func removeNodeNameOfPodsInSelectedNode(pods []*corev1.Pod, nodeName string) {
	for i, pod := range pods {
		if pod.Spec.NodeName != nodeName {
			continue
		}
		// skip pods owned by daemonset
		if !ownedByDaemonset(pod.OwnerReferences) {
			pods[i].Spec.NodeName = ""
		}
	}
}

func ownedByDaemonset(refs []metav1.OwnerReference) bool {
	for _, ref := range refs {
		if ref.Kind == simontype.DaemonSet {
			return true
		}
	}
	return false
}

func getClusterArgsForSimulation(nodes []*corev1.Node, pods []*corev1.Pod) simulator.ResourceTypes {
	return simulator.ResourceTypes{
		Nodes: nodes,
		Pods:  pods,
	}
}

func setNoScheduleTaintForNode(nodes []*corev1.Node, nodeName string) {
	for i, node := range nodes {
		if node.Name == nodeName {
			unschedulableTaint := corev1.Taint{
				Key:    corev1.TaintNodeUnschedulable,
				Effect: corev1.TaintEffectNoSchedule,
			}
			nodes[i].Spec.Taints = append(nodes[i].Spec.Taints, unschedulableTaint)
		}
	}
}

func taintExists(taint corev1.Taint, taints []corev1.Taint) bool {
	for _, t := range taints {
		if t == taint {
			return true
		}
	}

	return false
}

func updateByResult(result *simulator.SimulateResult) map[string]corev1.Pod {
	var rstPods map[string]corev1.Pod = make(map[string]corev1.Pod)
	for _, nodeStatus := range result.NodeStatus {
		for _, pod := range nodeStatus.Pods {
			rstPods[fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)] = *pod
		}
	}
	return rstPods
}
