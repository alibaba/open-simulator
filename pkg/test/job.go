package test

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FakeJobOption func(*batchv1.Job)

func MakeFakeJob(name, namespace string, completions int32, cpu, memory string, opts ...FakeJobOption) *batchv1.Job {
	res := corev1.ResourceList{}
	if cpu != "" {
		res[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		res[corev1.ResourceMemory] = resource.MustParse(memory)
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			Completions: &completions,
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
		opt(job)
	}

	return job
}

func WithJobTolerations(tolerations []corev1.Toleration) FakeJobOption {
	return func(job *batchv1.Job) {
		job.Spec.Template.Spec.Tolerations = tolerations
	}
}

func WithJobAffinity(affinity *corev1.Affinity) FakeJobOption {
	return func(job *batchv1.Job) {
		job.Spec.Template.Spec.Affinity = affinity
	}
}

func WithJobNodeSelector(nodeSelector map[string]string) FakeJobOption {
	return func(job *batchv1.Job) {
		job.Spec.Template.Spec.NodeSelector = nodeSelector
	}
}
