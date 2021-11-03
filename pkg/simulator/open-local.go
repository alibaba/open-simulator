package simulator

import (
	"context"

	simontype "github.com/alibaba/open-simulator/pkg/type"
	v1 "k8s.io/api/core/v1"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"
)

// SimonPlugin is a plugin for scheduling framework
type LocalPlugin struct {
	schedulerName string
}

var _ = framework.FilterPlugin(&LocalPlugin{})

// var _ = framework.ScorePlugin(&LocalPlugin{})

// Name returns name of the plugin. It is used in logs, etc.
func (plugin *LocalPlugin) Name() string {
	return simontype.SimonPluginName
}

// Score invoked at the score extension point.
func (plugin *LocalPlugin) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {

	// 检查是否pod有open-local pv，没有则直接退出。pv信息放在pod的annotation里面，主要包含容量和sc名称
	// pod pv信息由 simulator 处理sum

	// 有的话，获取节点上的LVM信息（也在anno中）。并调用open-local的相关函数

	if kind, exist := nodeInfo.Node().Annotations[simontype.AnnoWorkloadKind]; !exist {
		return framework.NewStatus(framework.Success)
	}

	// 测试的话，先把信息拿到先

	return framework.NewStatus(framework.Success)
}
