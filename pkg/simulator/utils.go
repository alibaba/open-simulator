package simulator

import (
	"context"

	simontype "github.com/alibaba/open-simulator/pkg/type"
	"github.com/alibaba/open-simulator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	externalclientset "k8s.io/client-go/kubernetes"
)

// GenerateValidPodsFromAppResources generate valid pods from resources
func GenerateValidPodsFromAppResources(client externalclientset.Interface, appname string, resources simontype.ResourceTypes) []*corev1.Pod {
	pods := make([]*corev1.Pod, 0)
	pods = append(pods, utils.GetValidPodExcludeDaemonSet(&resources)...)

	// DaemonSet will match with specific nodes so it needs to be handled separately
	var nodes []*corev1.Node

	// get all nodes
	nodeItems, _ := client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	for _, item := range nodeItems.Items {
		newItem := item
		nodes = append(nodes, &newItem)
	}

	// get all pods from daemonset
	for _, item := range resources.DaemonSets {
		newItem := item
		pods = append(pods, utils.MakeValidPodsByDaemonset(newItem, nodes)...)
	}

	// set label
	for _, pod := range pods {
		metav1.SetMetaDataLabel(&pod.ObjectMeta, simontype.LabelAppName, appname)
	}

	return pods
}
