package test

import (
	"encoding/json"

	simontype "github.com/alibaba/open-simulator/pkg/type"
	"github.com/alibaba/open-simulator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FakeNodeOption func(*corev1.Node)

func MakeFakeNode(name string, cpu, memory string, opts ...FakeNodeOption) *corev1.Node {
	res := corev1.ResourceList{}
	if cpu != "" {
		res[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		res[corev1.ResourceMemory] = resource.MustParse(memory)
	}
	res["pods"] = *resource.NewQuantity(110, resource.DecimalSI)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: corev1.NodeStatus{
			Capacity:    res,
			Allocatable: res,
		},
	}

	for _, opt := range opts {
		opt(node)
	}

	return node
}

// WithNodeAnnotations
func WithNodeAnnotations(annotations map[string]string) FakeNodeOption {
	return func(node *corev1.Node) {
		node.ObjectMeta.Annotations = annotations
	}
}

// WithNodeLabels
func WithNodeLabels(labels map[string]string) FakeNodeOption {
	return func(node *corev1.Node) {
		node.ObjectMeta.Labels = labels
	}
}

// WithNodeTaints
func WithNodeTaints(taints []corev1.Taint) FakeNodeOption {
	return func(node *corev1.Node) {
		node.Spec.Taints = taints
	}
}

// WithNodeLocalStorage
func WithNodeLocalStorage(storage utils.NodeStorage) FakeNodeOption {
	return func(node *corev1.Node) {
		b, _ := json.Marshal(storage)
		metav1.SetMetaDataAnnotation(&node.ObjectMeta, simontype.AnnoNodeLocalStorage, string(b))
	}
}
