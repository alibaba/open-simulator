package test

import (
	"encoding/json"

	simontype "github.com/alibaba/open-simulator/pkg/type"
	"github.com/alibaba/open-simulator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FakeStatefulSetOption func(*appsv1.StatefulSet)

func MakeFakeStatefulSet(name, namespace string, replica int32, cpu, memory string, opts ...FakeStatefulSetOption) *appsv1.StatefulSet {
	res := corev1.ResourceList{}
	if cpu != "" {
		res[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		res[corev1.ResourceMemory] = resource.MustParse(memory)
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replica,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "container",
							Image: "nginx",
							Resources: corev1.ResourceRequirements{
								Requests: res,
							},
						},
					},
				},
			},
		},
	}

	for _, opt := range opts {
		opt(sts)
	}

	return sts
}

func WithStatefulSetTolerations(tolerations []corev1.Toleration) FakeStatefulSetOption {
	return func(sts *appsv1.StatefulSet) {
		sts.Spec.Template.Spec.Tolerations = tolerations
	}
}

func WithStatefulSetAffinity(affinity *corev1.Affinity) FakeStatefulSetOption {
	return func(sts *appsv1.StatefulSet) {
		sts.Spec.Template.Spec.Affinity = affinity
	}
}

func WithStatefulSetNodeSelector(nodeSelector map[string]string) FakeStatefulSetOption {
	return func(sts *appsv1.StatefulSet) {
		sts.Spec.Template.Spec.NodeSelector = nodeSelector
	}
}

func WithStatefulSetLocalStorage(volumes utils.VolumeRequest) FakeStatefulSetOption {
	return func(sts *appsv1.StatefulSet) {
		b, _ := json.Marshal(volumes)
		metav1.SetMetaDataAnnotation(&sts.Spec.Template.ObjectMeta, simontype.AnnoPodLocalStorage, string(b))
	}
}
