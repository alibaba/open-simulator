package plugin

import (
	"context"
	"fmt"
	"math"

	"github.com/pquerna/ffjson/ffjson"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	storagev1informers "k8s.io/client-go/informers/storage/v1"
	externalclientset "k8s.io/client-go/kubernetes"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"

	localtype "github.com/alibaba/open-local/pkg"
	localalgorithm "github.com/alibaba/open-local/pkg/scheduler/algorithm"
	localalgo "github.com/alibaba/open-local/pkg/scheduler/algorithm/algo"
	localpriorities "github.com/alibaba/open-local/pkg/scheduler/algorithm/priorities"

	simontype "github.com/alibaba/open-simulator/pkg/type"
	"github.com/alibaba/open-simulator/pkg/utils"
)

// LocalPlugin is a plugin for scheduling framework
type LocalPlugin struct {
	schedulerName string
	fakeclient    externalclientset.Interface
	// open-local need storageInformer to get sc
	storageInformer storagev1informers.Interface
}

var _ = framework.FilterPlugin(&LocalPlugin{})
var _ = framework.ScorePlugin(&LocalPlugin{})
var _ = framework.BindPlugin(&LocalPlugin{})

// NewLocalPlugin
func NewLocalPlugin(schedulerName string, fakeclient externalclientset.Interface, storageInformers storagev1informers.Interface, configuration runtime.Object, f framework.Handle) (framework.Plugin, error) {
	return &LocalPlugin{
		schedulerName:   schedulerName,
		storageInformer: storageInformers,
		fakeclient:      fakeclient,
	}, nil
}

// Name returns name of the plugin. It is used in logs, etc.
func (plugin *LocalPlugin) Name() string {
	return simontype.OpenLocalPluginName
}

// Score invoked at the score extension point.
func (plugin *LocalPlugin) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	// check if the pod needs storage resources
	lvmPVCs, devicePVCs := utils.GetPodLocalPVCs(pod)
	if len(lvmPVCs)+len(devicePVCs) == 0 {
		// the node is scheduable if pod does not require storage resources
		return framework.NewStatus(framework.Success)
	}

	// check if the node have storage resources
	node := nodeInfo.Node()
	nc := utils.GetNodeCache(node)
	if nc == nil {
		// the node is unscheduable when:
		// 1. node does not have storage resources
		// 2. pod does not require storage resources
		return framework.NewStatus(framework.Unschedulable)
	}

	// create SchedulingContext
	schedulingContext := localalgorithm.NewSchedulingContext(nil, plugin.storageInformer, nil, nil, localtype.NewNodeAntiAffinityWeight())
	schedulingContext.ClusterNodeCache.Nodes[nodeInfo.Node().Name] = nc

	// process Open-Local LVM
	fits, _, err := localalgo.ProcessLVMPVCPredicate(lvmPVCs, nodeInfo.Node(), schedulingContext)
	if !fits {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}

	// process Open-Local Device
	fits, _, err = localalgo.ProcessDevicePVC(nil, devicePVCs, nodeInfo.Node(), schedulingContext)
	if !fits {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}

	return framework.NewStatus(framework.Success)
}

func (plugin *LocalPlugin) Score(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	node, _ := plugin.fakeclient.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	// check if the pod needs storage resources
	nodeExist, podExist := true, true
	lvmPVCs, devicePVCs := utils.GetPodLocalPVCs(pod)
	log.Debugf("score lvmPVCs %d, devicePVCs %d\n", len(lvmPVCs), len(devicePVCs))
	if len(lvmPVCs)+len(devicePVCs) == 0 {
		podExist = false
	}

	// check if the node have storage resources
	nc := utils.GetNodeCache(node)
	if nc == nil {
		nodeExist = false
	}

	// there are 3 situations that need to be dealt with in advance:
	// MaxScore: node doesn't have storage resources and pod doesn't require storage resources
	// Unschedulable: node doesn't have storage resources but pod requires storage resources
	// MinScore: node has storage resources but pod doesn't require storage resources
	if !nodeExist {
		if !podExist {
			return int64(localpriorities.MaxScore), framework.NewStatus(framework.Success)
		} else {
			return int64(localpriorities.MinScore), framework.NewStatus(framework.Unschedulable, fmt.Sprintf("no local storage found in node %s", node.Name))
		}
	} else {
		if !podExist {
			return int64(localpriorities.MinScore), framework.NewStatus(framework.Success)
		}
	}

	// create SchedulingContext
	schedulingContext := localalgorithm.NewSchedulingContext(nil, plugin.storageInformer, nil, nil, localtype.NewNodeAntiAffinityWeight())
	schedulingContext.ClusterNodeCache.Nodes[node.Name] = nc

	// process Open-Local LVM
	score1, _, err := localalgo.ScoreLVMVolume(nil, lvmPVCs, node, schedulingContext)
	if err != nil {
		return int64(localpriorities.MinScore), framework.NewStatus(framework.Error, err.Error())
	}

	// process Open-Local Device
	score2, _, err := localalgo.ScoreDeviceVolume(nil, devicePVCs, node, schedulingContext)
	if err != nil {
		return int64(localpriorities.MinScore), framework.NewStatus(framework.Error, err.Error())
	}

	return int64(score1 + score2), framework.NewStatus(framework.Success)
}

// ScoreExtensions of the Score plugin.
func (plugin *LocalPlugin) ScoreExtensions() framework.ScoreExtensions {
	return plugin
}

// NormalizeScore invoked after scoring all nodes.
func (plugin *LocalPlugin) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, scores framework.NodeScoreList) *framework.Status {
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

// Bind invoked at the bind extension point.
// LocalPlugin Bind must be executed before SimonPlugin Bind
func (plugin *LocalPlugin) Bind(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	node, _ := plugin.fakeclient.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	// check if the pod needs storage resources
	nodeExist, podExist := true, true
	lvmPVCs, devicePVCs := utils.GetPodLocalPVCs(pod)
	if len(lvmPVCs)+len(devicePVCs) == 0 {
		podExist = false
	}

	// check if the node have storage resources
	nc := utils.GetNodeCache(node)
	if nc == nil {
		nodeExist = false
	}

	if !nodeExist {
		return framework.NewStatus(framework.Skip)
	}
	if !podExist {
		return framework.NewStatus(framework.Skip)
	}

	// create SchedulingContext
	schedulingContext := localalgorithm.NewSchedulingContext(nil, plugin.storageInformer, nil, nil, localtype.NewNodeAntiAffinityWeight())
	schedulingContext.ClusterNodeCache.Nodes[node.Name] = nc

	// process Open-Local LVM
	_, lvmUnits, err := localalgo.ScoreLVMVolume(nil, lvmPVCs, node, schedulingContext)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	// process Open-Local Device
	_, deviceUnits, err := localalgo.ScoreDeviceVolume(nil, devicePVCs, node, schedulingContext)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	log.Debugf("bind  units1 %d, units2 %d\n", len(lvmUnits), len(deviceUnits))

	units := append(lvmUnits, deviceUnits...)

	// update annotation in node
	nodeStorage := utils.GetNodeStorage(node)
	for i := range units {
		if units[i].VolumeType == localtype.VolumeTypeLVM {
			for j := range nodeStorage.VGs {
				if nodeStorage.VGs[j].Name == units[i].VgName {
					nodeStorage.VGs[j].Requested += units[i].Requested
					break
				}
			}
		} else if units[i].VolumeType == localtype.VolumeTypeDevice || units[i].VolumeType == localtype.VolumeTypeMountPoint {
			for j := range nodeStorage.Devices {
				if nodeStorage.Devices[j].Name == units[i].Device {
					nodeStorage.Devices[j].IsAllocated = true
					break
				}
			}
		}
	}
	if data, err := ffjson.Marshal(nodeStorage); err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	} else {
		metav1.SetMetaDataAnnotation(&node.ObjectMeta, simontype.AnnoNodeLocalStorage, string(data))
	}

	// update Node
	if _, err := plugin.fakeclient.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{}); err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	// should always skip
	return framework.NewStatus(framework.Skip)
}
