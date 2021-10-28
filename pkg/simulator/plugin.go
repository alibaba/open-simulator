package simulator

import (
	"context"
	"fmt"
	"math"
	"os"

	"github.com/alibaba/open-simulator/pkg/algo"
	simontype "github.com/alibaba/open-simulator/pkg/type"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	resourcehelper "k8s.io/kubectl/pkg/util/resource"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"
)

// SimonPlugin is a plugin for scheduling framework
type SimonPlugin struct {
	schedulerName string
	sim           *Simulator
}

var _ = framework.ScorePlugin(&SimonPlugin{})
var _ = framework.BindPlugin(&SimonPlugin{})

// Name returns name of the plugin. It is used in logs, etc.
func (plugin *SimonPlugin) Name() string {
	return simontype.SimonPluginName
}

// Bind invoked at the bind extension point.
func (plugin *SimonPlugin) Bind(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	return plugin.sim.BindPodToNode(ctx, state, pod, nodeName, plugin.schedulerName)
}

// Score invoked at the score extension point.
func (plugin *SimonPlugin) Score(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	podReq, _ := resourcehelper.PodRequestsAndLimits(pod)
	if len(podReq) == 0 {
		return framework.MaxNodeScore, framework.NewStatus(framework.Success)
	}

	node, err := plugin.sim.fakeClient.CoreV1().Nodes().Get(plugin.sim.ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("get node %s failed: %s\n", nodeName, err.Error())
		os.Exit(1)
	}

	res := float64(0)
	for resourceName := range node.Status.Allocatable {
		podAllocatedRes := podReq[resourceName]
		nodeAvailableRes := node.Status.Allocatable[resourceName]
		nodeAvailableRes.Sub(podAllocatedRes)
		share := algo.Share(podAllocatedRes.AsApproximateFloat64(), nodeAvailableRes.AsApproximateFloat64())
		if share > res {
			res = share
		}
	}

	return int64(float64((framework.MaxNodeScore - framework.MinNodeScore)) * res), framework.NewStatus(framework.Success)
}

// // ScoreExtensions of the Score plugin.
func (plugin *SimonPlugin) ScoreExtensions() framework.ScoreExtensions {
	return plugin
}

// NormalizeScore invoked after scoring all nodes.
func (plugin *SimonPlugin) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, scores framework.NodeScoreList) *framework.Status {
	// Find highest and lowest scores.
	var highest int64 = -math.MaxInt64
	var lowest int64 = math.MaxInt64
	for _, nodeScore := range scores {
		if nodeScore.Score > highest {
			highest = nodeScore.Score
		}
		if nodeScore.Score < lowest {
			lowest = nodeScore.Score
		}
	}

	// Transform the highest to lowest score range to fit the framework's min to max node score range.
	oldRange := highest - lowest
	newRange := framework.MaxNodeScore - framework.MinNodeScore
	for i, nodeScore := range scores {
		if oldRange == 0 {
			scores[i].Score = framework.MinNodeScore
		} else {
			scores[i].Score = ((nodeScore.Score - lowest) * newRange / oldRange) + framework.MinNodeScore
		}
	}

	return framework.NewStatus(framework.Success)
}
