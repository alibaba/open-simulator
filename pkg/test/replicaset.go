package test

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FakeReplicaSetOption func(*appsv1.ReplicaSet)

func MakeFakeReplicaSet(name, namespace string, replica int32, cpu, memory string, opts ...FakeReplicaSetOption) *appsv1.ReplicaSet {
	res := corev1.ResourceList{}
	if cpu != "" {
		res[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		res[corev1.ResourceMemory] = resource.MustParse(memory)
	}

	replicaset := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.ReplicaSetSpec{
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
		opt(replicaset)
	}

	return replicaset
}

func WithReplicaSetTolerations(tolerations []corev1.Toleration) FakeReplicaSetOption {
	return func(replicaset *appsv1.ReplicaSet) {
		replicaset.Spec.Template.Spec.Tolerations = tolerations
	}
}

func WithReplicaSetAffinity(affinity *corev1.Affinity) FakeReplicaSetOption {
	return func(replicaset *appsv1.ReplicaSet) {
		replicaset.Spec.Template.Spec.Affinity = affinity
	}
}

func WithReplicaSetNodeSelector(nodeSelector map[string]string) FakeReplicaSetOption {
	return func(replicaset *appsv1.ReplicaSet) {
		replicaset.Spec.Template.Spec.NodeSelector = nodeSelector
	}
}
