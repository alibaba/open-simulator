package test

import (
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FakeCronJobOption func(*batchv1beta1.CronJob)

func MakeCronFakeJob(name, namespace string, completions int32, cpu, memory string, opts ...FakeCronJobOption) *batchv1beta1.CronJob {
	res := corev1.ResourceList{}
	if cpu != "" {
		res[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		res[corev1.ResourceMemory] = resource.MustParse(memory)
	}

	cronjob := &batchv1beta1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1beta1.CronJobSpec{
			JobTemplate: batchv1beta1.JobTemplateSpec{
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
			},
		},
	}

	for _, opt := range opts {
		opt(cronjob)
	}

	return cronjob
}

func WithCronJobTolerations(tolerations []corev1.Toleration) FakeCronJobOption {
	return func(cronjob *batchv1beta1.CronJob) {
		cronjob.Spec.JobTemplate.Spec.Template.Spec.Tolerations = tolerations
	}
}

func WithCronJobAffinity(affinity *corev1.Affinity) FakeCronJobOption {
	return func(cronjob *batchv1beta1.CronJob) {
		cronjob.Spec.JobTemplate.Spec.Template.Spec.Affinity = affinity
	}
}

func WithCronJobNodeSelector(nodeSelector map[string]string) FakeCronJobOption {
	return func(cronjob *batchv1beta1.CronJob) {
		cronjob.Spec.JobTemplate.Spec.Template.Spec.NodeSelector = nodeSelector
	}
}
