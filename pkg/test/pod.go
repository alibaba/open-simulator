package test

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FakePodOption func(*corev1.Pod)

func MakeFakePod(name, namespace string, cpu, memory string, opts ...FakePodOption) *corev1.Pod {
	res := corev1.ResourceList{}
	if cpu != "" {
		res[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		res[corev1.ResourceMemory] = resource.MustParse(memory)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
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
	}

	for _, opt := range opts {
		opt(pod)
	}

	return pod
}

func WithPodAnnotations(annotations map[string]string) FakePodOption {
	return func(pod *corev1.Pod) {
		pod.ObjectMeta.Annotations = annotations
	}
}

func WithPodLabels(labels map[string]string) FakePodOption {
	return func(pod *corev1.Pod) {
		pod.ObjectMeta.Labels = labels
	}
}

func WithPodNodeName(nodeName string) FakePodOption {
	return func(pod *corev1.Pod) {
		pod.Spec.NodeName = nodeName
	}
}

func WithPodTolerations(tolerations []corev1.Toleration) FakePodOption {
	return func(pod *corev1.Pod) {
		pod.Spec.Tolerations = tolerations
	}
}

func WithPodAffinity(affinity *corev1.Affinity) FakePodOption {
	return func(pod *corev1.Pod) {
		pod.Spec.Affinity = affinity
	}
}

func WithPodNodeSelector(nodeSelector map[string]string) FakePodOption {
	return func(pod *corev1.Pod) {
		pod.Spec.NodeSelector = nodeSelector
	}
}
