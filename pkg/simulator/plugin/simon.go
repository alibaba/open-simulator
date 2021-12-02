package plugin

import (
	"context"
	"fmt"
	"github.com/alibaba/open-simulator/pkg/algo"
	simontype "github.com/alibaba/open-simulator/pkg/type"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	externalclientset "k8s.io/client-go/kubernetes"
	resourcehelper "k8s.io/kubectl/pkg/util/resource"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"
	"math"
)

// SimonPlugin is a plugin for scheduling framework
type SimonPlugin struct {
	fakeclient externalclientset.Interface
}

var _ = framework.ScorePlugin(&SimonPlugin{})
var _ = framework.BindPlugin(&SimonPlugin{})

func NewSimonPlugin(fakeclient externalclientset.Interface, configuration runtime.Object, f framework.Handle) (framework.Plugin, error) {
	return &SimonPlugin{
		fakeclient: fakeclient,
	}, nil
}

// Name returns name of the plugin. It is used in logs, etc.
func (plugin *SimonPlugin) Name() string {
	return simontype.SimonPluginName
}

// Bind invoked at the bind extension point.
func (plugin *SimonPlugin) Bind(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	return plugin.BindPodToNode(ctx, state, pod, nodeName)
}

// Score invoked at the score extension point.
func (plugin *SimonPlugin) Score(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	podReq, _ := resourcehelper.PodRequestsAndLimits(pod)
	if len(podReq) == 0 {
		return framework.MaxNodeScore, framework.NewStatus(framework.Success)
	}

	node, err := plugin.fakeclient.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	if err != nil {
		return framework.MinNodeScore, framework.NewStatus(framework.Error, fmt.Sprintf("Score | %v", err))
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

// ScoreExtensions of the Score plugin.
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

// BindPodToNode bind pod to a node and trigger pod update event
func (plugin *SimonPlugin) BindPodToNode(ctx context.Context, state *framework.CycleState, p *corev1.Pod, nodeName string) *framework.Status {
	// fmt.Printf("bind pod %s/%s to node %s\n", p.Namespace, p.Name, nodeName)
	// Step 1: update pod info
	pod, err := plugin.fakeclient.CoreV1().Pods(p.Namespace).Get(context.TODO(), p.Name, metav1.GetOptions{})
	if err != nil {
		log.Errorf("BindPodToNode | get pod error %v", err)
		return framework.NewStatus(framework.Error, fmt.Sprintf("BindPodToNode | unable to bind: %v", err))
	}
	updatedPod := pod.DeepCopy()
	updatedPod.Spec.NodeName = nodeName
	updatedPod.Status.Phase = corev1.PodRunning

	// Step 2: update pod
	// here assuming the pod is already in the resource storage
	// so the update is needed to emit update event in case a handler is registered
	_, err = plugin.fakeclient.CoreV1().Pods(pod.Namespace).Update(context.TODO(), updatedPod, metav1.UpdateOptions{})
	if err != nil {
		log.Errorf("BindPodToNode | update error %v", err)
		return framework.NewStatus(framework.Error, fmt.Sprintf("BindPodToNode | unable to add new pod: %v", err))
	}

	return nil
}
