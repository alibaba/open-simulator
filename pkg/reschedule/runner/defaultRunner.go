package runner

import (
	"fmt"
	"github.com/alibaba/open-simulator/pkg/simulator"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sort"
)

type DefaultRunner struct {
}

type PodPlan struct {
	PodName      string
	PodNamespace string
	FromNode     string
	ToNode       string
	PodOwnerRefs []metav1.OwnerReference
}

type ReschedulePlan struct {
	Node     string
	PodPlans []PodPlan
}

type Runner interface {
	Run(allNodes []corev1.Node, allPods []corev1.Pod) ([]ReschedulePlan, error)
}

func NewDefaultRunner() Runner {
	return &DefaultRunner{}
}

func (runner DefaultRunner) Run(allNodes []corev1.Node, allPods []corev1.Pod) ([]ReschedulePlan, error) {
	_, workers := GetWorkersAndMasters(allNodes)
	if len(workers) <= 1 {
		return nil, nil
	}

	srcLayout := BuildMapForNodesPods(allNodes, allPods)
	dstLayout := DeepCopyLayoutForNodesPods(srcLayout)

	removableWorkerNames := make([]string, 0)
	for i := 0; i < len(workers); i++ {
		tmpLayout := DeepCopyLayoutForNodesPods(dstLayout)

		// step 1: select one node to be offline
		removableWorker := captureOneRemovableWorker(tmpLayout)
		if removableWorker == nil {
			break
		}

		preProcessRemovableWorkerBeforeReschedule(removableWorker, tmpLayout)

		rst, err := simulator.Simulate(getClusterArgsForSimulation(tmpLayout), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to simulate schedule: %v", err)
		}

		if len(rst.UnscheduledPods) != 0 {
			for k := range dstLayout {
				if k.Name == removableWorker.Name {
					k.Labels["simon.io/non-removable"] = ""
				}
			}
			continue
		} else {
			removableWorkerNames = append(removableWorkerNames, removableWorker.Name)
			dstLayout = updateLayoutByResult(rst.NodeStatus)
		}
	}

	migrationPlans := makeMigrationPlan(removableWorkerNames, dstLayout)

	return migrationPlans, nil
}

func captureOneRemovableWorker(layout map[*corev1.Node]PodSlice) *corev1.Node {
	var minScore int64 = 200
	var minScoreWorker *corev1.Node = nil
	for node, podSlice := range layout {
		if !fitNode(node, podSlice) {
			continue
		}

		if len(layout[node]) == 0 {
			return node
		}

		reqs := GetPodsTotalRequestsExcludeStaticAndDaemonPod(layout[node])
		nodeCpuReq, nodeMemoryReq, _ :=
			reqs[corev1.ResourceCPU], reqs[corev1.ResourceMemory], reqs[corev1.ResourceEphemeralStorage]
		allocatable := node.Status.Allocatable
		nodeFractionCpuReq := float64(nodeCpuReq.MilliValue()) / float64(allocatable.Cpu().MilliValue()) * 100
		nodeFractionMemoryReq := float64(nodeMemoryReq.Value()) / float64(allocatable.Memory().Value()) * 100
		if (int64(nodeFractionCpuReq) + int64(nodeFractionMemoryReq)) < minScore {
			minScoreWorker = node
			minScore = int64(nodeFractionCpuReq) + int64(nodeFractionMemoryReq)
		}
	}

	return minScoreWorker
}

// TODO: filter the worker that has the designated pod
func fitNode(node *corev1.Node, pods []*corev1.Pod) bool {
	if _, exist := node.Labels["node-role.kubernetes.io/master"]; exist {
		//fmt.Printf("Non-removable(%s): not a worker\n", node.Name)
		return false
	}

	if _, exist := node.Labels["simon.io/non-removable"]; exist {
		//fmt.Printf("Non-removable(%s): unsuccessful rescheduling \n", node.Name)
		return false
	}

	if exist := TaintExists(corev1.Taint{
		Key:    corev1.TaintNodeUnschedulable,
		Effect: corev1.TaintEffectNoSchedule,
	}, node.Spec.Taints); exist {
		//fmt.Printf("Non-removable(%s): unschedulable node\n", node.Name)
		return false
	}

	return true
}

func preProcessRemovableWorkerBeforeReschedule(worker *corev1.Node, layout map[*corev1.Node]PodSlice) {
	layout[worker] = RemoveDaemonPod(layout[worker])
	AddOriginatedFromWhichNodeAnnotation(layout[worker])
	InitNodeNameOfPodsOnNode(layout[worker])
	SetNoScheduleTaintOnNode(worker)
}

func getClusterArgsForSimulation(layout map[*corev1.Node]PodSlice) simulator.ResourceTypes {
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

func updateLayoutByResult(nodeStatus []simulator.NodeStatus) map[*corev1.Node]PodSlice {
	newLayout := make(map[*corev1.Node]PodSlice)
	for _, status := range nodeStatus {
		newLayout[status.Node] = status.Pods
	}
	return newLayout
}

func makeMigrationPlan(removableWorkerNames []string, dstLayout map[*corev1.Node]PodSlice) []ReschedulePlan {
	var migrationPlans []ReschedulePlan
	// init
	for _, workerName := range removableWorkerNames {
		migrationPlan := ReschedulePlan{
			Node:     workerName,
			PodPlans: make([]PodPlan, 0),
		}
		migrationPlans = append(migrationPlans, migrationPlan)
	}

	// collect migrated pods
	var migratedPods []*corev1.Pod
	for _, pods := range dstLayout {
		for _, pod := range pods {
			if _, exist := pod.Annotations["originated-from"]; exist {
				migratedPods = append(migratedPods, pod)
			}
		}
	}

	for _, pod := range migratedPods {
		for i := range migrationPlans {
			if migrationPlans[i].Node == pod.Annotations["originated-from"] {
				podPlan := PodPlan{
					PodName:      pod.Name,
					PodNamespace: pod.Namespace,
					FromNode:     pod.Annotations["originated-from"],
					ToNode:       pod.Spec.NodeName,
					PodOwnerRefs: pod.OwnerReferences,
				}
				migrationPlans[i].PodPlans = append(migrationPlans[i].PodPlans, podPlan)
				break
			}
		}
	}

	// sort
	for n := range migrationPlans {
		sort.Slice(
			migrationPlans[n].PodPlans,
			func(i, j int) bool {
				if migrationPlans[n].PodPlans[i].PodNamespace < migrationPlans[n].PodPlans[j].PodNamespace {
					return true
				}
				if migrationPlans[n].PodPlans[i].PodNamespace == migrationPlans[n].PodPlans[j].PodNamespace {
					return migrationPlans[n].PodPlans[i].PodName < migrationPlans[n].PodPlans[j].PodName
				}
				return false
			},
		)
	}

	return migrationPlans
}
