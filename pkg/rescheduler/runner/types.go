package runner

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodPlan struct {
	PodName      string
	PodNamespace string
	FromNode     string
	ToNode       string
	PodOwnerRefs []metav1.OwnerReference
}

type MigrationPlan struct {
	NodeName string
	PodPlans []PodPlan
}

type Runner interface {
	Run(allNodes []corev1.Node, allPods []corev1.Pod) ([]MigrationPlan, error)
}
