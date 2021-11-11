package simontype

import (
	apps "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	v1 "k8s.io/api/storage/v1"
)

type ResourceInfo struct {
	Name     string
	Resource ResourceTypes
}

type ResourceTypes struct {
	Nodes                  []*corev1.Node
	Pods                   []*corev1.Pod
	DaemonSets             []*apps.DaemonSet
	StatefulSets           []*apps.StatefulSet
	Deployments            []*apps.Deployment
	ReplicationControllers []*corev1.ReplicationController
	ReplicaSets            []*apps.ReplicaSet
	Services               []*corev1.Service
	PersistentVolumeClaims []*corev1.PersistentVolumeClaim
	StorageClasss          []*v1.StorageClass
	PodDisruptionBudgets   []*v1beta1.PodDisruptionBudget
	Jobs                   []*batchv1.Job
	CronJobs               []*batchv1beta1.CronJob
}
