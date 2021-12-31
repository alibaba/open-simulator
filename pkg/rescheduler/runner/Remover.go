package runner

import (
	"fmt"
	"github.com/alibaba/open-simulator/pkg/rescheduler/runner/utils"
	"github.com/alibaba/open-simulator/pkg/simulator"

	corev1 "k8s.io/api/core/v1"
	"sort"
)

type RemoverOptions struct {
	LabelFilter               []string
	MaximumAverageUtilization int
}

type Remover struct {
	labelFilter               []string
	MaximumAverageUtilization int
}

func NewRemover(opts RemoverOptions) Runner {
	return &Remover{
		labelFilter: opts.LabelFilter,
	}
}

// TODO: Pod 检验、所剩worker的数量、资源水位
func (r Remover) Run(allNodes []corev1.Node, allPods []corev1.Pod) ([]MigrationPlan, error) {
	_, workers := utils.GetMastersAndWorkers(allNodes)
	if len(workers) <= 1 {
		return nil, nil
	}

	normalizedNodes, normalizedPods, err := utils.NormalizePodsNodes(allNodes, allPods)
	if err != nil {
		return nil, err
	}

	srcLayout := utils.BuildMapForNodesPods(normalizedNodes, normalizedPods)
	dstLayout := utils.DeepCopyLayoutForNodesPods(srcLayout)

	removableWorkerNames := make([]string, 0)
	for i := 0; i < len(workers)-1; i++ {
		// TODO: we stop to take nodes offline when the resource average utilization reaches the setting

		tmpLayout := utils.DeepCopyLayoutForNodesPods(dstLayout)

		removableWorker := r.captureOneRemovableWorker(tmpLayout)
		if removableWorker == nil {
			break
		}

		r.preProcessRemovableWorkerBeforeReschedule(removableWorker, tmpLayout)

		rst, err := simulator.Simulate(utils.GetClusterArgsForSimulation(tmpLayout), nil)
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
			dstLayout = utils.UpdateLayoutByResult(rst.NodeStatus)
		}
	}

	migrationPlans := r.makeMigrationPlan(removableWorkerNames, dstLayout)

	return migrationPlans, nil
}

func (r Remover) captureOneRemovableWorker(layout map[*corev1.Node]utils.PodSlice) *corev1.Node {
	var minScore int64 = 200
	var minScoreWorker *corev1.Node = nil
	for node, podSlice := range layout {
		if !r.fitNode(node, podSlice) {
			continue
		}

		if len(layout[node]) == 0 {
			return node
		}

		reqs := utils.GetPodsTotalRequestsExcludeStaticAndDaemonPod(layout[node])
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

// TODO: filter the worker that has the designated pod by label
func (r Remover) fitNode(node *corev1.Node, pods []*corev1.Pod) bool {
	if _, exist := node.Labels["node-role.kubernetes.io/master"]; exist {
		//fmt.Printf("Non-removable(%s): not a worker\n", node.Name)
		return false
	}

	if _, exist := node.Labels["simon.io/non-removable"]; exist {
		//fmt.Printf("Non-removable(%s): unsuccessful rescheduling \n", node.Name)
		return false
	}

	if exist := utils.TaintExists(corev1.Taint{
		Key:    corev1.TaintNodeUnschedulable,
		Effect: corev1.TaintEffectNoSchedule,
	}, node.Spec.Taints); exist {
		//fmt.Printf("Non-removable(%s): unschedulable node\n", node.Name)
		return false
	}

	return true
}

func (r Remover) preProcessRemovableWorkerBeforeReschedule(worker *corev1.Node, layout map[*corev1.Node]utils.PodSlice) {
	layout[worker] = utils.RemoveDaemonAndStaticPod(layout[worker])
	utils.AddOriginatedFromWhichNodeAnnotation(layout[worker])
	utils.InitNodeNameOfPodsOnNode(layout[worker])
	utils.SetNoScheduleTaintOnNode(worker)
}

func (r Remover) makeMigrationPlan(removableWorkerNames []string, dstLayout map[*corev1.Node]utils.PodSlice) []MigrationPlan {
	var migrationPlans []MigrationPlan
	// init
	for _, workerName := range removableWorkerNames {
		migrationPlan := MigrationPlan{
			NodeName: workerName,
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
			if migrationPlans[i].NodeName == pod.Annotations["originated-from"] {
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
