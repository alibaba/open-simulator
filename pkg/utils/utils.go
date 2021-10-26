package utils

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	simontype "github.com/alibaba/open-simulator/pkg/type"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	v1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/events"
	resourcehelper "k8s.io/kubectl/pkg/util/resource"
	schedconfig "k8s.io/kubernetes/cmd/kube-scheduler/app/config"
	schedoptions "k8s.io/kubernetes/cmd/kube-scheduler/app/options"
	api "k8s.io/kubernetes/pkg/apis/core"
	apiv1 "k8s.io/kubernetes/pkg/apis/core/v1"
	"k8s.io/kubernetes/pkg/apis/core/validation"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/daemon"
	"k8s.io/kubernetes/pkg/scheduler/profile"
)

const (
	DaemonSetFromCluster = "daemonset-from-cluster"
	FakeNode             = "fake-node"
)

// ParseFilePath converts recursively directory path to a slice of file paths
func ParseFilePath(path string, filePaths *[]string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	switch mode := fi.Mode(); {
	case mode.IsDir():
		files, err := ioutil.ReadDir(path)
		if err != nil {
			return err
		}
		for _, f := range files {
			p := filepath.Join(path, f.Name())
			err = ParseFilePath(p, filePaths)
			if err != nil {
				return err
			}
		}
	case mode.IsRegular():
		*filePaths = append(*filePaths, path)
		return nil
	default:
		return fmt.Errorf("invalid path: %s", path)
	}

	return nil
}

// GetObjectsFromFiles converts yml or yaml file to kubernetes resources
func GetObjectsFromFiles(filePaths []string) (simontype.ResourceTypes, error) {
	var resources simontype.ResourceTypes

	for _, f := range filePaths {
		obj := DecodeYamlFile(f)
		switch o := obj.(type) {
		case nil:
			continue
		case *corev1.Node:
			resources.Nodes = append(resources.Nodes, o)
		case *corev1.Pod:
			resources.Pods = append(resources.Pods, o)
		case *apps.DaemonSet:
			resources.DaemonSets = append(resources.DaemonSets, o)
		case *apps.StatefulSet:
			resources.StatefulSets = append(resources.StatefulSets, o)
		case *apps.Deployment:
			resources.Deployments = append(resources.Deployments, o)
		case *corev1.Service:
			resources.Services = append(resources.Services, o)
		case *corev1.PersistentVolumeClaim:
			resources.PersistentVolumeClaims = append(resources.PersistentVolumeClaims, o)
		case *corev1.ReplicationController:
			resources.ReplicationControllers = append(resources.ReplicationControllers, o)
		case *apps.ReplicaSet:
			resources.ReplicaSets = append(resources.ReplicaSets, o)
		case *v1.StorageClass:
			resources.StorageClasss = append(resources.StorageClasss, o)
		case *v1beta1.PodDisruptionBudget:
			resources.PodDisruptionBudgets = append(resources.PodDisruptionBudgets, o)
		default:
			fmt.Printf("unknown type: %T\n", o)
			continue
		}
	}
	return resources, nil
}

// DecodeYamlFile captures the yml or yaml file, and decodes it
func DecodeYamlFile(file string) runtime.Object {
	fileExtension := filepath.Ext(file)
	if fileExtension != ".yaml" && fileExtension != ".yml" {
		return nil
	}
	yamlFile, err := ioutil.ReadFile(file)
	if err != nil {
		fmt.Printf("Error while read file %s: %s\n", file, err.Error())
		os.Exit(1)
	}

	decode := scheme.Codecs.UniversalDeserializer().Decode
	obj, _, err := decode(yamlFile, nil, nil)

	if err != nil {
		fmt.Printf("Error while decoding YAML object. Err was: %s", err)
		os.Exit(1)
	}

	return obj
}

func GetMasterFromKubeConfig(filename string) (string, error) {
	config, err := clientcmd.LoadFromFile(filename)
	if err != nil {
		return "", fmt.Errorf("can not load kubeconfig file: %v", err)
	}

	context, ok := config.Contexts[config.CurrentContext]
	if !ok {
		return "", fmt.Errorf("Failed to get master address from kubeconfig")
	}

	if val, ok := config.Clusters[context.Cluster]; ok {
		return val.Server, nil
	}
	return "", fmt.Errorf("Failed to get master address from kubeconfig")
}

func InitKubeSchedulerConfiguration(opts *schedoptions.Options) (*schedconfig.CompletedConfig, error) {
	c := &schedconfig.Config{}
	// clear out all unnecessary options so no port is bound
	// to allow running multiple instances in a row
	opts.Deprecated = nil
	opts.CombinedInsecureServing = nil
	opts.SecureServing = nil
	if err := opts.ApplyTo(c); err != nil {
		return nil, fmt.Errorf("unable to get scheduler config: %v", err)
	}

	// Get the completed config
	cc := c.Complete()

	// completely ignore the events
	cc.EventBroadcaster = events.NewEventBroadcasterAdapter(fakeclientset.NewSimpleClientset())

	return &cc, nil
}

func GetRecorderFactory(cc *schedconfig.CompletedConfig) profile.RecorderFactory {
	return func(name string) events.EventRecorder {
		return cc.EventBroadcaster.NewRecorder(name)
	}
}

// GetValidPodExcludeDaemonSet gets valid pod by resources exclude DaemonSet that needs to be handled specially
func GetValidPodExcludeDaemonSet(resources *simontype.ResourceTypes) {

	//get valid pods by pods
	for i, item := range resources.Pods {
		resources.Pods[i] = MakeValidPodByPod(item)
	}

	// get all pods from deployment
	for _, deploy := range resources.Deployments {
		resources.Pods = append(resources.Pods, MakeValidPodsByDeployment(deploy)...)
	}

	// get all pods from statefulset
	for _, sts := range resources.StatefulSets {
		resources.Pods = append(resources.Pods, MakeValidPodsByStatefulSet(sts)...)
	}
}

func MakeValidPodsByDeployment(deploy *apps.Deployment) []*corev1.Pod {
	var pods []*corev1.Pod
	if deploy.Spec.Replicas == nil {
		var replica int32 = 1
		deploy.Spec.Replicas = &replica
	}
	for ordinal := 0; ordinal < int(*deploy.Spec.Replicas); ordinal++ {
		pod, _ := controller.GetPodFromTemplate(&deploy.Spec.Template, deploy, nil)
		pod.ObjectMeta.Name = fmt.Sprintf("fake-deployment-%s-%d", deploy.Name, ordinal)
		pod.ObjectMeta.Namespace = deploy.Namespace
		pod = MakePodValid(pod)
		pod = AddWorkloadInfoToPod(pod, simontype.WorkloadKindDeployment, deploy.Name, pod.Namespace)
		pods = append(pods, pod)
	}
	return pods
}

func MakeValidPodsByStatefulSet(set *apps.StatefulSet) []*corev1.Pod {
	var pods []*corev1.Pod
	if set.Spec.Replicas == nil {
		var replica int32 = 1
		set.Spec.Replicas = &replica
	}
	for ordinal := 0; ordinal < int(*set.Spec.Replicas); ordinal++ {
		pod, _ := controller.GetPodFromTemplate(&set.Spec.Template, set, nil)
		pod.ObjectMeta.Name = fmt.Sprintf("fake-statefulset-%s-%d", set.Name, ordinal)
		pod.ObjectMeta.Namespace = set.Namespace
		pod = MakePodValid(pod)
		pod = AddWorkloadInfoToPod(pod, simontype.WorkloadKindStatefulSet, set.Name, pod.Namespace)
		pods = append(pods, pod)
	}
	return pods
}

func MakeValidPodsByDaemonset(ds *apps.DaemonSet, nodes []*corev1.Node) []*corev1.Pod {
	var pods []*corev1.Pod
	for _, node := range nodes {
		pod := NewDaemonPod(ds, node.Name)
		shouldRun := NodeShouldRunDaemonPod(node, pod)
		if shouldRun {
			pods = append(pods, pod)
		}
	}
	return pods
}

func NodeShouldRunDaemonPod(node *corev1.Node, pod *corev1.Pod) bool {
	taints := node.Spec.Taints
	fitsNodeName, fitsNodeAffinity, fitsTaints := daemon.Predicates(pod, node, taints)
	if !fitsNodeName || !fitsNodeAffinity || !fitsTaints {
		return false
	}
	return true
}

func NewDaemonPod(ds *apps.DaemonSet, nodeName string) *corev1.Pod {
	pod, _ := controller.GetPodFromTemplate(&ds.Spec.Template, ds, nil)
	pod.ObjectMeta.Name = fmt.Sprintf("fake-daemonset-%s-%s", ds.Name, nodeName)
	pod.ObjectMeta.Namespace = ds.Namespace
	pod = MakePodValid(pod)
	pod = AddWorkloadInfoToPod(pod, simontype.WorkloadKindDaemonSet, ds.Name, pod.Namespace)
	pod.Spec.NodeName = nodeName
	return pod
}

func MakeValidPodByPod(pod *corev1.Pod) *corev1.Pod {
	return MakePodValid(pod)
}

func MakePodValid(oldPod *corev1.Pod) *corev1.Pod {
	newPod := oldPod.DeepCopy()
	if newPod.ObjectMeta.Namespace == "" {
		newPod.ObjectMeta.Namespace = corev1.NamespaceDefault
	}
	newPod.ObjectMeta.UID = uuid.NewUUID()
	if newPod.Labels == nil {
		newPod.Labels = make(map[string]string)
	}
	if newPod.Spec.InitContainers != nil {
		for i := range newPod.Spec.InitContainers {
			if newPod.Spec.InitContainers[i].TerminationMessagePolicy == "" {
				newPod.Spec.InitContainers[i].TerminationMessagePolicy = corev1.TerminationMessageFallbackToLogsOnError
			}
			if newPod.Spec.InitContainers[i].ImagePullPolicy == "" {
				newPod.Spec.InitContainers[i].ImagePullPolicy = corev1.PullIfNotPresent
			}
			if newPod.Spec.InitContainers[i].SecurityContext != nil && newPod.Spec.InitContainers[i].SecurityContext.Privileged != nil {
				var priv = false
				newPod.Spec.InitContainers[i].SecurityContext.Privileged = &priv
			}
		}
	}
	if newPod.Spec.Containers != nil {
		for i := range newPod.Spec.Containers {
			if newPod.Spec.Containers[i].TerminationMessagePolicy == "" {
				newPod.Spec.Containers[i].TerminationMessagePolicy = corev1.TerminationMessageFallbackToLogsOnError
			}
			if newPod.Spec.Containers[i].ImagePullPolicy == "" {
				newPod.Spec.Containers[i].ImagePullPolicy = corev1.PullIfNotPresent
			}
			if newPod.Spec.Containers[i].SecurityContext != nil && newPod.Spec.Containers[i].SecurityContext.Privileged != nil {
				var priv = false
				newPod.Spec.Containers[i].SecurityContext.Privileged = &priv
			}
			newPod.Spec.Containers[i].VolumeMounts = nil
		}
	}

	if newPod.Spec.DNSPolicy == "" {
		newPod.Spec.DNSPolicy = corev1.DNSClusterFirst
	}
	if newPod.Spec.RestartPolicy == "" {
		newPod.Spec.RestartPolicy = corev1.RestartPolicyAlways
	}
	if newPod.Spec.SchedulerName == "" {
		newPod.Spec.SchedulerName = simontype.DefaultSchedulerName
	}
	// Add pod provisioner annotation
	if newPod.ObjectMeta.Annotations == nil {
		newPod.ObjectMeta.Annotations = map[string]string{}
	}
	newPod.ObjectMeta.Annotations[simontype.AnnoPodProvisioner] = simontype.DefaultSchedulerName
	newPod.ObjectMeta.Annotations[simontype.AnnoFake] = ""
	// todo: handle pvc
	if !ValidatePod(newPod) {
		os.Exit(1)
	}

	return newPod
}

func AddWorkloadInfoToPod(pod *corev1.Pod, kind string, name string, namespace string) *corev1.Pod {
	pod.ObjectMeta.Annotations[simontype.AnnoWorkloadKind] = kind
	pod.ObjectMeta.Annotations[simontype.AnnoWorkloadName] = name
	pod.ObjectMeta.Annotations[simontype.AnnoWorkloadNamespace] = namespace
	return pod
}

func MakeValidNodeByNode(node *corev1.Node, nodename string) *corev1.Node {
	node.ObjectMeta.Name = nodename
	if node.ObjectMeta.Labels == nil {
		node.ObjectMeta.Labels = map[string]string{}
	}
	node.ObjectMeta.Labels[corev1.LabelHostname] = nodename
	if node.ObjectMeta.Annotations == nil {
		node.ObjectMeta.Annotations = map[string]string{}
	}
	node.ObjectMeta.Annotations[simontype.AnnoFake] = ""
	node.ObjectMeta.UID = uuid.NewUUID()
	if !ValidateNode(node) {
		os.Exit(1)
	}
	return node
}

func ValidatePod(pod *corev1.Pod) bool {
	internalPod := &api.Pod{}
	if err := apiv1.Convert_v1_Pod_To_core_Pod(pod, internalPod, nil); err != nil {
		fmt.Printf("unable to convert to internal version: %#v", err)
		return false
	}
	if errs := validation.ValidatePodCreate(internalPod, validation.PodValidationOptions{}); len(errs) > 0 {
		var errStrs []string
		for _, err := range errs {
			errStrs = append(errStrs, fmt.Sprintf("%v: %v", err.Type, err.Field))
		}
		fmt.Printf("Invalid pod: %#v", strings.Join(errStrs, ", "))
		return false
	}
	return true
}

func ValidateNode(node *corev1.Node) bool {
	internalNode := &api.Node{}
	if err := apiv1.Convert_v1_Node_To_core_Node(node, internalNode, nil); err != nil {
		fmt.Printf("unable to convert to internal version: %#v", err)
		return false
	}
	if errs := validation.ValidateNode(internalNode); len(errs) > 0 {
		var errStrs []string
		for _, err := range errs {
			errStrs = append(errStrs, fmt.Sprintf("%v: %v", err.Type, err.Field))
		}
		fmt.Printf("Invalid node: %#v", strings.Join(errStrs, ", "))
		return false
	}
	return true
}

func GetPodsTotalRequestsAndLimitsByNodeName(pods []corev1.Pod, nodeName string) (reqs map[corev1.ResourceName]resource.Quantity, limits map[corev1.ResourceName]resource.Quantity) {
	reqs, limits = map[corev1.ResourceName]resource.Quantity{}, map[corev1.ResourceName]resource.Quantity{}
	for _, pod := range pods {
		if pod.Spec.NodeName != nodeName {
			continue
		}
		podReqs, podLimits := resourcehelper.PodRequestsAndLimits(&pod)
		for podReqName, podReqValue := range podReqs {
			if value, ok := reqs[podReqName]; !ok {
				reqs[podReqName] = podReqValue.DeepCopy()
			} else {
				value.Add(podReqValue)
				reqs[podReqName] = value
			}
		}
		for podLimitName, podLimitValue := range podLimits {
			if value, ok := limits[podLimitName]; !ok {
				limits[podLimitName] = podLimitValue.DeepCopy()
			} else {
				value.Add(podLimitValue)
				limits[podLimitName] = value
			}
		}
	}
	return
}

func GetNodePodsCount(podList *corev1.PodList, nodeName string) (count int64) {
	count = 0
	for _, pod := range podList.Items {
		if pod.Spec.NodeName == nodeName {
			count++
		}
	}
	return
}

func IsFake(anno map[string]string) bool {
	_, fake := anno[simontype.AnnoFake]
	return fake
}

func AdjustWorkloads(workloads map[string][]string) {
	if workloads == nil {
		return
	}

	for name, nodes := range workloads {
		workloads[name] = AdjustNodesOrder(nodes)
	}
}

func AdjustNodesOrder(nodes []string) []string {
	queue := NewNodeQueue(nodes)
	sort.Sort(queue)
	return queue.nodes
}

type NodeQueue struct {
	nodes []string
}

func NewNodeQueue(nodes []string) *NodeQueue {
	return &NodeQueue{
		nodes: nodes,
	}
}

func (queue *NodeQueue) Len() int { return len(queue.nodes) }
func (queue *NodeQueue) Swap(i, j int) {
	queue.nodes[i], queue.nodes[j] = queue.nodes[j], queue.nodes[i]
}
func (queue *NodeQueue) Less(i, j int) bool {
	if strings.Contains(queue.nodes[i], simontype.FakeNodeNamePrefix) && strings.Contains(queue.nodes[j], simontype.FakeNodeNamePrefix) {
		return queue.nodes[i] < queue.nodes[j]
	}

	if !strings.Contains(queue.nodes[i], simontype.FakeNodeNamePrefix) && !strings.Contains(queue.nodes[j], simontype.FakeNodeNamePrefix) {
		return queue.nodes[i] < queue.nodes[j]
	}

	if !strings.Contains(queue.nodes[i], simontype.FakeNodeNamePrefix) {
		return true
	}

	if !strings.Contains(queue.nodes[j], simontype.FakeNodeNamePrefix) {
		return false
	}

	return true
}

// MultiplyMilliQuant scales quantity by factor
func MultiplyMilliQuant(quant resource.Quantity, factor float64) resource.Quantity {
	milliValue := quant.MilliValue()
	newMilliValue := int64(float64(milliValue) * factor)
	newQuant := resource.NewMilliQuantity(newMilliValue, quant.Format)
	return *newQuant
}

// MultiplyQuant scales quantity by factor
func MultiplyQuant(quant resource.Quantity, factor float64) resource.Quantity {
	value := quant.Value()
	newValue := int64(float64(value) * factor)
	newQuant := resource.NewQuantity(newValue, quant.Format)
	return *newQuant
}

func GetNodeAllocatable(node *corev1.Node) (resource.Quantity, resource.Quantity) {
	nodeAllocatable := node.Status.Allocatable.DeepCopy()
	return *nodeAllocatable.Cpu(), *nodeAllocatable.Memory()
}
