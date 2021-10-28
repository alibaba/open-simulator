package simontype

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AliQoSClass string

const (
	SimonPluginName      = "Simon"
	NewNodeNamePrefix    = "simon"
	DefaultSchedulerName = corev1.DefaultSchedulerName

	StopReasonSuccess = "everything is ok"

	AnnoPodProvisioner    = "simon/provisioned-by"
	AnnoWorkloadKind      = "simon/workload-kind"
	AnnoWorkloadName      = "simon/workload-name"
	AnnoWorkloadNamespace = "simon/workload-namespace"

	WorkloadKindDeployment  = "Deployment"
	WorkloadKindStatefulSet = "StatefulSet"
	WorkloadKindDaemonSet   = "DaemonSet"

	ConfigMapName      = "simulator-plan"
	ConfigMapNamespace = metav1.NamespaceSystem
	ConfigMapFileName  = "configmap-simon.yaml"
)
