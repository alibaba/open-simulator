package test

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FakeDeploymentOption func(*appsv1.Deployment)

func MakeFakeDeployment(name, namespace string, replica int32, cpu, memory string, opts ...FakeDeploymentOption) *appsv1.Deployment {
	res := corev1.ResourceList{}
	if cpu != "" {
		res[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		res[corev1.ResourceMemory] = resource.MustParse(memory)
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
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
		opt(deploy)
	}

	return deploy
}

func WithDeploymentTolerations(tolerations []corev1.Toleration) FakeDeploymentOption {
	return func(deploy *appsv1.Deployment) {
		deploy.Spec.Template.Spec.Tolerations = tolerations
	}
}

func WithDeploymentAffinity(affinity *corev1.Affinity) FakeDeploymentOption {
	return func(deploy *appsv1.Deployment) {
		deploy.Spec.Template.Spec.Affinity = affinity
	}
}

func WithDeploymentNodeSelector(nodeSelector map[string]string) FakeDeploymentOption {
	return func(deploy *appsv1.Deployment) {
		deploy.Spec.Template.Spec.NodeSelector = nodeSelector
	}
}
