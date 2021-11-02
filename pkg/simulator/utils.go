package simulator

import (
	"context"

	simontype "github.com/alibaba/open-simulator/pkg/type"
	"github.com/alibaba/open-simulator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	externalclientset "k8s.io/client-go/kubernetes"
)

// GenerateValidPodsFromResources generate valid pods from resources
func GenerateValidPodsFromResources(client externalclientset.Interface, resources simontype.ResourceTypes) ([]*corev1.Pod, error) {
	pods := make([]*corev1.Pod, 0)
	pods = append(pods, utils.GetValidPodExcludeDaemonSet(&resources)...)

	// DaemonSet will match with specific nodes so it needs to be handled separately
	var nodes []*corev1.Node
	var fakeNodes []*corev1.Node

	// get all nodes
	nodeItems, _ := client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	for _, item := range nodeItems.Items {
		newItem := item
		nodes = append(nodes, &newItem)
	}
	// get all fake nodes
	nodeItems, _ = client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{LabelSelector: simontype.LabelNewNode})
	for _, item := range nodeItems.Items {
		newItem := item
		fakeNodes = append(fakeNodes, &newItem)
	}

	// get all pods from daemonset
	daemonsets, _ := client.AppsV1().DaemonSets(corev1.NamespaceAll).List(context.Background(), metav1.ListOptions{LabelSelector: simontype.LabelDaemonSetFromCluster})
	for _, item := range daemonsets.Items {
		newItem := item
		pods = append(pods, utils.MakeValidPodsByDaemonset(&newItem, fakeNodes)...)
	}
	for _, item := range resources.DaemonSets {
		newItem := item
		pods = append(pods, utils.MakeValidPodsByDaemonset(newItem, nodes)...)
	}

	// set label
	for _, pod := range pods {
		metav1.SetMetaDataLabel(&pod.ObjectMeta, simontype.LabelNewPod, "")
	}

	return pods, nil
}
