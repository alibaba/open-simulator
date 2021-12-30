package simontype

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	SimonPluginName        = "Simon"
	OpenLocalPluginName    = "Open-Local"
	OpenGpuSharePluginName = "Open-Gpu-Share"
	NewNodeNamePrefix      = "simon"
	DefaultSchedulerName   = corev1.DefaultSchedulerName

	StopReasonSuccess   = "everything is ok"
	StopReasonDoNotStop = "do not stop"
	CreatePodError      = "failed to create pod"
	DeletePodError      = "failed to delete pod"

	AnnoWorkloadKind      = "simon/workload-kind"
	AnnoWorkloadName      = "simon/workload-name"
	AnnoWorkloadNamespace = "simon/workload-namespace"
	AnnoNodeLocalStorage  = "simon/node-local-storage"
	AnnoPodLocalStorage   = "simon/pod-local-storage"
	AnnoNodeGpuShare      = "simon/node-gpu-share"
	AnnoPodGpuShare       = "simon/pod-gpu-share"

	LabelNewNode = "simon/new-node"
	LabelAppName = "simon/app-name"

	EnvMaxCPU    = "MaxCPU"
	EnvMaxMemory = "MaxMemory"
	EnvMaxVG     = "MaxVG"

	Pod                   = "Pod"
	Deployment            = "Deployment"
	ReplicaSet            = "ReplicaSet"
	ReplicationController = "ReplicationController"
	StatefulSet           = "StatefulSet"
	DaemonSet             = "DaemonSet"
	Job                   = "Job"
	CronJob               = "CronJob"

	ConfigMapName      = "simulator-plan"
	ConfigMapNamespace = metav1.NamespaceSystem
	ConfigMapFileName  = "configmap-simon.yaml"

	NotesFileSuffix       = "NOTES.txt"
	SeparateSymbol        = "-"
	WorkLoadHashCodeDigit = 10
	PodHashCodeDigit      = 5
	MaxNumNewNode         = 100
)
