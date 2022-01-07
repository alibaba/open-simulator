package migrator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alibaba/open-simulator/pkg/migrate/migrator/utils"
	"github.com/alibaba/open-simulator/pkg/simulator"
	corev1 "k8s.io/api/core/v1"
)

type downScalerOptions struct {
	nodesToBeRemoved          []string
	labelFilter               []string
	maximumAverageUtilization int
}

type option func(opts *downScalerOptions)

type DownScaler struct {
	NodesToBeRemoved          []string
	LabelFilter               []string
	MaximumAverageUtilization int
}

func NewDownScaler(opts ...option) *DownScaler {
	var downScalerOpts downScalerOptions
	for _, opt := range opts {
		opt(&downScalerOpts)
	}

	return &DownScaler{
		NodesToBeRemoved:          downScalerOpts.nodesToBeRemoved,
		LabelFilter:               downScalerOpts.labelFilter,
		MaximumAverageUtilization: downScalerOpts.maximumAverageUtilization,
	}
}

func NewMigratorOnDownScaler(opts ...option) Migrator {
	return NewDownScaler(opts...)
}

func ScaleDownCluster(clusterResources simulator.ResourceTypes, removeList []string, opts ...option) (MigrationResult, error) {
	var nodesMigrationStatus []MigrationStatus

	d := NewDownScaler(opts...)

	normalizedNodes, normalizedPods, err := utils.NormalizePodsNodes(clusterResources.Nodes, clusterResources.Pods)
	if err != nil {
		return MigrationResult{}, err
	}

	srcLayout, dstLayout := d.BuildLayoutForNodesPods(normalizedNodes, normalizedPods)

	ineligibleNodeWithReason := d.SelectIneligibleNodeWithReason(removeList, d.LabelFilter, srcLayout)

	for nodeName, reason := range ineligibleNodeWithReason {
		nonRemovableNodeStatus := MigrationStatus{
			NodeName:    nodeName,
			IsRemovable: false,
			Reason:      reason,
		}
		nodesMigrationStatus = append(nodesMigrationStatus, nonRemovableNodeStatus)
		removeList = utils.DeleteElemInStringSlice(nodeName, removeList)
	}

	removableWorkerNames := make([]string, 0)
	for {
		// TODO: we stop to take nodes offline when the resource average utilization reaches the setting
		if len(removeList) == 0 {
			break
		}

		tmpLayout := utils.DeepCopyLayoutForNodesPods(dstLayout)

		nodeTobeRemoved := d.SelectTheMinimumResourceUtilizationNode(removeList, tmpLayout)

		d.PreProcessRemovableWorkerBeforeMigrating(nodeTobeRemoved, tmpLayout)

		rst, err := simulator.Simulate(utils.GetClusterArgsForSimulation(tmpLayout), nil)
		if err != nil {
			return MigrationResult{}, fmt.Errorf("failed to simulate schedule: %v", err)
		}

		if len(rst.UnscheduledPods) != 0 {
			var reason string
			for _, unScheduledPod := range rst.UnscheduledPods {
				reason = fmt.Sprintf("%s\n%s", unScheduledPod.Reason, reason)
			}
			newMigrationStatus := MigrationStatus{
				NodeName:    nodeTobeRemoved.Name,
				IsRemovable: false,
				Reason:      reason,
			}
			nodesMigrationStatus = append(nodesMigrationStatus, newMigrationStatus)
		} else {
			removableWorkerNames = append(removableWorkerNames, nodeTobeRemoved.Name)
			dstLayout = utils.UpdateLayoutByResult(rst.NodeStatus)
		}

		removeList = utils.DeleteElemInStringSlice(nodeTobeRemoved.Name, removeList)
	}

	nodesMigrationStatus = append(nodesMigrationStatus, d.MakeMigrationStatusForRemovableNode(removableWorkerNames, dstLayout)...)

	return MigrationResult{nodesMigrationStatus}, nil
}

func (d *DownScaler) SelectIneligibleNodeWithReason(nodesToBeRemoved []string, labelFilters []string, layout map[*corev1.Node][]*corev1.Pod) map[string]string {
	ineligibleNodeWithReason := make(map[string]string)
	for _, nodeName := range nodesToBeRemoved {
		for node, podSlice := range layout {
			if node.Name == nodeName {
				if fit, reason := d.IsRemovableNode(node, podSlice, labelFilters); !fit {
					ineligibleNodeWithReason[nodeName] = reason
				}
				break
			}
		}
	}
	return ineligibleNodeWithReason
}

func (d *DownScaler) IsRemovableNode(node *corev1.Node, podSlice []*corev1.Pod, labelFilters []string) (bool, string) {
	var reason string
	if _, exist := node.Labels["node-role.kubernetes.io/master"]; exist {
		reason = fmt.Sprintf("\tNot a worker\n%s", reason)
	}

	if exist := utils.TaintExists(corev1.Taint{
		Key:    corev1.TaintNodeUnschedulable,
		Effect: corev1.TaintEffectNoSchedule,
	}, node.Spec.Taints); exist {
		reason = fmt.Sprintf("\tExist unschedulable taint\n%s", reason)
	}

	if len(labelFilters) != 0 {
		mapLabelFilters := make(map[string]string)
		for _, label := range labelFilters {
			l := strings.SplitN(label, "=", 2)
			mapLabelFilters[l[0]] = l[1]
		}

		for _, pod := range podSlice {
			if pod.Labels != nil {
				var equalLabels []string
				for k1, v1 := range pod.Labels {
					if v2, exist := mapLabelFilters[k1]; exist {
						if v1 == v2 {
							equalLabels = append(equalLabels, fmt.Sprintf("%s=%s", k1, v1))
						}
					}
				}
				if len(equalLabels) != 0 {
					reason = fmt.Sprintf("\tThe pod(%s/%s) exists label(%s)\n%s", pod.Namespace, pod.Name, strings.Join(equalLabels, ";"), reason)
				}
			}
		}
	}

	if reason != "" {
		return false, reason
	}

	return true, reason
}

func (d *DownScaler) SelectTheMinimumResourceUtilizationNode(nodeNames []string, layout map[*corev1.Node][]*corev1.Pod) *corev1.Node {
	var minScore int64 = 200
	var minScoreWorker *corev1.Node
	for _, nodeName := range nodeNames {
		for node, podSlice := range layout {
			if node.Name == nodeName {
				if len(podSlice) == 0 {
					return node
				}

				reqs := utils.GetPodsTotalRequestsExcludeStaticAndDaemonPod(podSlice)
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
		}
	}

	return minScoreWorker
}

// TODO: 所剩worker的数量、资源水位
func (d *DownScaler) Migrate(clusterResources simulator.ResourceTypes) (MigrationResult, error) {
	return ScaleDownCluster(
		clusterResources,
		d.MakeRemoveList(clusterResources.Nodes),
		WithLabelFilter(d.LabelFilter),
		WithMaximumAverageUtilization(d.MaximumAverageUtilization),
	)
}

func (d *DownScaler) MakeRemoveList(nodes []*corev1.Node) []string {
	var removeList []string
	removeList = append(removeList, d.NodesToBeRemoved...)
	if len(removeList) == 0 {
		for _, node := range nodes {
			removeList = append(removeList, node.Name)
		}
	}

	return removeList
}

func (d *DownScaler) BuildLayoutForNodesPods(nodes []*corev1.Node, pods []*corev1.Pod) (
	srcLayout map[*corev1.Node][]*corev1.Pod,
	dstLayout map[*corev1.Node][]*corev1.Pod,
) {
	srcLayout = utils.BuildMapForNodesPods(nodes, pods)
	dstLayout = utils.DeepCopyLayoutForNodesPods(srcLayout)

	return srcLayout, dstLayout
}

func (d *DownScaler) PreProcessRemovableWorkerBeforeMigrating(worker *corev1.Node, layout map[*corev1.Node][]*corev1.Pod) {
	layout[worker] = utils.RemoveDaemonAndStaticPod(layout[worker])
	utils.AddOriginatedFromWhichNodeAnnotation(layout[worker])
	utils.InitNodeNameOfPodsOnNode(layout[worker])
	utils.SetNoScheduleTaintOnNode(worker)
}

func (d *DownScaler) MakeMigrationStatusForRemovableNode(removableNodeNames []string, dstLayout map[*corev1.Node][]*corev1.Pod) []MigrationStatus {
	var nodesMigrationStatus []MigrationStatus
	// init
	for _, nodeName := range removableNodeNames {
		newMigrationStatus := MigrationStatus{
			NodeName:    nodeName,
			IsRemovable: true,
			PodPlans:    make([]PodPlan, 0),
		}
		nodesMigrationStatus = append(nodesMigrationStatus, newMigrationStatus)
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

	// make plan
	for _, pod := range migratedPods {
		for i := range nodesMigrationStatus {
			if nodesMigrationStatus[i].NodeName == pod.Annotations["originated-from"] {
				podPlan := PodPlan{
					PodName:      pod.Name,
					PodNamespace: pod.Namespace,
					FromNode:     pod.Annotations["originated-from"],
					ToNode:       pod.Spec.NodeName,
					PodOwnerRefs: pod.OwnerReferences,
				}
				nodesMigrationStatus[i].PodPlans = append(nodesMigrationStatus[i].PodPlans, podPlan)
				break
			}
		}
	}

	// sort
	for n := range nodesMigrationStatus {
		sort.Slice(
			nodesMigrationStatus[n].PodPlans,
			func(i, j int) bool {
				if nodesMigrationStatus[n].PodPlans[i].PodNamespace < nodesMigrationStatus[n].PodPlans[j].PodNamespace {
					return true
				}
				if nodesMigrationStatus[n].PodPlans[i].PodNamespace == nodesMigrationStatus[n].PodPlans[j].PodNamespace {
					return nodesMigrationStatus[n].PodPlans[i].PodName < nodesMigrationStatus[n].PodPlans[j].PodName
				}
				return false
			},
		)
	}

	return nodesMigrationStatus
}

func WithNodesToBeRemoved(nodesTobeRemoved []string) option {
	return func(opts *downScalerOptions) {
		opts.nodesToBeRemoved = nodesTobeRemoved
	}
}

func WithLabelFilter(labelFilter []string) option {
	return func(opts *downScalerOptions) {
		opts.labelFilter = labelFilter
	}
}

func WithMaximumAverageUtilization(maximumAverageUtilization int) option {
	return func(opts *downScalerOptions) {
		opts.maximumAverageUtilization = maximumAverageUtilization
	}
}
