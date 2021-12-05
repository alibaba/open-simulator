package test

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FakeDaemonSetOption func(*appsv1.DaemonSet)

func MakeFakeDaemonSet(name, namespace string, cpu, memory string, opts ...FakeDaemonSetOption) *appsv1.DaemonSet {
	res := corev1.ResourceList{}
	if cpu != "" {
		res[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		res[corev1.ResourceMemory] = resource.MustParse(memory)
	}

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DaemonSetSpec{
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
		opt(ds)
	}

	return ds
}

func WithDaemonSetTolerations(tolerations []corev1.Toleration) FakeDaemonSetOption {
	return func(ds *appsv1.DaemonSet) {
		ds.Spec.Template.Spec.Tolerations = tolerations
	}
}

func WithDaemonSetAffinity(affinity *corev1.Affinity) FakeDaemonSetOption {
	return func(ds *appsv1.DaemonSet) {
		ds.Spec.Template.Spec.Affinity = affinity
	}
}

func WithDaemonSetNodeSelector(nodeSelector map[string]string) FakeDaemonSetOption {
	return func(ds *appsv1.DaemonSet) {
		ds.Spec.Template.Spec.NodeSelector = nodeSelector
	}
}
