package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	localcache "github.com/alibaba/open-local/pkg/scheduler/algorithm/cache"
	localutils "github.com/alibaba/open-local/pkg/utils"
	simontype "github.com/alibaba/open-simulator/pkg/type"
	"github.com/pquerna/ffjson/ffjson"
	log "github.com/sirupsen/logrus"
	"helm.sh/helm/v3/pkg/releaseutil"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	v1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// ParseFilePath converts recursively directory path to a slice of file paths
func ParseFilePath(path string) (filePaths []string, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	switch mode := fi.Mode(); {
	case mode.IsDir():
		files, err := ioutil.ReadDir(path)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			p := filepath.Join(path, f.Name())
			subFiles, err := ParseFilePath(p)
			if err != nil {
				return nil, err
			}
			filePaths = append(filePaths, subFiles...)
		}
	case mode.IsRegular():
		filePaths = append(filePaths, path)
		return filePaths, nil
	default:
		return nil, fmt.Errorf("invalid path: %s", path)
	}

	return filePaths, nil
}

// GetObjectsFromFiles converts yml or yaml file to kubernetes resources
func GetObjectsFromFiles(filePaths []string) (simontype.ResourceTypes, error) {
	var resources simontype.ResourceTypes

	for _, f := range filePaths {
		objects := DecodeYamlFile(f)
		for _, obj := range objects {
			switch o := obj.(type) {
			case nil:
				continue
			case *corev1.Node:
				resources.Nodes = append(resources.Nodes, o)
				storageFile := fmt.Sprintf("%s.json", strings.TrimSuffix(f, filepath.Ext(f)))
				if err := AddLocalStorageInfoInNode(o, storageFile); err != nil && !errors.Is(err, os.ErrNotExist) {
					return resources, err
				}
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
	}
	return resources, nil
}

// DecodeYamlFile captures the yml or yaml file, and decodes it
func DecodeYamlFile(file string) []runtime.Object {
	fileExtension := filepath.Ext(file)
	if fileExtension != ".yaml" && fileExtension != ".yml" {
		return nil
	}
	yamlFile, err := ioutil.ReadFile(file)
	if err != nil {
		fmt.Printf("Error while read file %s: %s\n", file, err.Error())
		os.Exit(1)
	}

	objects := make([]runtime.Object, 0)
	yamls := releaseutil.SplitManifests(string(yamlFile))
	for _, yaml := range yamls {
		decode := scheme.Codecs.UniversalDeserializer().Decode
		obj, _, err := decode([]byte(yaml), nil, nil)
		if err != nil {
			fmt.Printf("Error while decoding YAML object. Err was: %s", err)
			os.Exit(1)
		}

		objects = append(objects, obj)
	}

	return objects
}

func ReadJsonFile(file string) (string, error) {
	if _, err := os.Stat(file); err != nil {
		return "", nil
	}

	plan, _ := ioutil.ReadFile(file)
	if ok := json.Valid(plan); !ok {
		return "", fmt.Errorf("valid json file %s failed", file)
	}
	return string(plan), nil
}

func AddLocalStorageInfoInNode(node *corev1.Node, jsonfile string) error {
	info, err := ReadJsonFile(jsonfile)
	if err != nil {
		return err
	}
	if info != "" {
		metav1.SetMetaDataAnnotation(&node.ObjectMeta, simontype.AnnoNodeLocalStorage, info)
	}
	return nil
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
func GetValidPodExcludeDaemonSet(resources *simontype.ResourceTypes) []*corev1.Pod {
	var pods []*corev1.Pod = make([]*corev1.Pod, 0)
	//get valid pods by pods
	for _, item := range resources.Pods {
		pods = append(pods, MakeValidPodByPod(item))
	}

	// get all pods from deployment
	for _, deploy := range resources.Deployments {
		pods = append(pods, MakeValidPodsByDeployment(deploy)...)
	}

	// get all pods from statefulset
	for _, sts := range resources.StatefulSets {
		pods = append(pods, MakeValidPodsByStatefulSet(sts)...)
	}

	return pods
}

func MakeValidPodsByDeployment(deploy *apps.Deployment) []*corev1.Pod {
	var pods []*corev1.Pod
	if deploy.Spec.Replicas == nil {
		var replica int32 = 1
		deploy.Spec.Replicas = &replica
	}
	for ordinal := 0; ordinal < int(*deploy.Spec.Replicas); ordinal++ {
		pod, _ := controller.GetPodFromTemplate(&deploy.Spec.Template, deploy, nil)
		pod.ObjectMeta.Name = fmt.Sprintf("deployment-%s-%d", deploy.Name, ordinal)
		pod.ObjectMeta.Namespace = deploy.Namespace
		pod = MakePodValid(pod)
		pod = AddWorkloadInfoToPod(pod, simontype.WorkloadKindDeployment, deploy.Name, pod.Namespace)
		pods = append(pods, pod)
	}
	return pods
}

type FakeNodeStorage struct {
	VGs     []localcache.SharedResource    `json:"vgs"`
	Devices []localcache.ExclusiveResource `json:"devices"`
}

type Volume struct {
	Size int64 `json:"size,string"`
	// Kind 可以是 LVM 或 HDD 或 SSD
	// HDD 和 SSD 均指代独占盘
	Kind string `json:"kind"`
}

type VolumeRequest struct {
	Volumes []Volume `json:"volumes"`
}

func GetNodeStorage(node *corev1.Node) *FakeNodeStorage {
	nodeStorageStr, exist := node.Annotations[simontype.AnnoNodeLocalStorage]
	if !exist {
		return nil
	}

	nodeStorage := new(FakeNodeStorage)
	if err := ffjson.Unmarshal([]byte(nodeStorageStr), nodeStorage); err != nil {
		log.Errorf("unmarshal info of node %s failed: %s", node.Name, err.Error())
		return nil
	}

	return nodeStorage
}

func GetNodeCache(node *corev1.Node) *localcache.NodeCache {
	nodeStorage := GetNodeStorage(node)
	if nodeStorage == nil {
		return nil
	}

	nc := localcache.NewNodeCache(node.Name)
	var vgRes map[localcache.ResourceName]localcache.SharedResource = make(map[localcache.ResourceName]localcache.SharedResource)
	for _, vg := range nodeStorage.VGs {
		vgRes[localcache.ResourceName(vg.Name)] = vg
	}
	nc.VGs = vgRes

	var deviceRes map[localcache.ResourceName]localcache.ExclusiveResource = make(map[localcache.ResourceName]localcache.ExclusiveResource)
	for _, device := range nodeStorage.Devices {
		deviceRes[localcache.ResourceName(device.Device)] = device
	}
	nc.Devices = deviceRes

	return nc
}

func GetPodStorage(pod *corev1.Pod) *VolumeRequest {
	podStorageStr, exist := pod.Annotations[simontype.AnnoPodLocalStorage]
	if !exist {
		return nil
	}

	podStorage := new(VolumeRequest)
	if err := ffjson.Unmarshal([]byte(podStorageStr), &podStorage); err != nil {
		log.Errorf("unmarshal volume info of pod %s/%s failed: %s", pod.Namespace, pod.Name, err.Error())
		return nil
	}

	return podStorage
}

func GetPodLocalPVCs(pod *corev1.Pod) ([]*corev1.PersistentVolumeClaim, []*corev1.PersistentVolumeClaim) {
	podStorage := GetPodStorage(pod)
	if podStorage == nil {
		return nil, nil
	}

	lvmPVCs := make([]*corev1.PersistentVolumeClaim, 0)
	devicePVCs := make([]*corev1.PersistentVolumeClaim, 0)
	for i, volume := range podStorage.Volumes {
		scName := ""
		if volume.Kind == "LVM" {
			scName = OpenLocalSCNameLVM
		} else if volume.Kind == "HDD" {
			scName = OpenLocalSCNameDeviceHDD
		} else if volume.Kind == "SSD" {
			scName = OpenLocalSCNameDeviceSSD
		} else {
			continue
		}
		volumeQuantity := resource.NewQuantity(volume.Size, resource.BinarySI)
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pvc-%s-%d", pod.Name, i),
				Namespace: pod.Namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				StorageClassName: &scName,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceName(corev1.ResourceStorage): *volumeQuantity,
					},
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimPending,
			},
		}
		if scName == OpenLocalSCNameLVM {
			lvmPVCs = append(lvmPVCs, pvc)
		} else {
			devicePVCs = append(devicePVCs, pvc)
		}
	}

	return lvmPVCs, devicePVCs
}

func MakeValidPodsByStatefulSet(set *apps.StatefulSet) []*corev1.Pod {
	var pods []*corev1.Pod
	if set.Spec.Replicas == nil {
		var replica int32 = 1
		set.Spec.Replicas = &replica
	}

	// handle Open-Local Volumes
	var volumes VolumeRequest
	volumes.Volumes = make([]Volume, 0)
	for _, pvc := range set.Spec.VolumeClaimTemplates {
		if *pvc.Spec.StorageClassName == OpenLocalSCNameLVM || *pvc.Spec.StorageClassName == YodaSCNameLVM {
			volume := Volume{
				Size: localutils.GetPVCRequested(&pvc),
				Kind: "LVM",
			}
			volumes.Volumes = append(volumes.Volumes, volume)
		} else if *pvc.Spec.StorageClassName == OpenLocalSCNameDeviceSSD || *pvc.Spec.StorageClassName == OpenLocalSCNameMountPointSSD || *pvc.Spec.StorageClassName == YodaSCNameMountPointSSD || *pvc.Spec.StorageClassName == YodaSCNameDeviceSSD {
			volume := Volume{
				Size: localutils.GetPVCRequested(&pvc),
				Kind: "SSD",
			}
			volumes.Volumes = append(volumes.Volumes, volume)
		} else if *pvc.Spec.StorageClassName == OpenLocalSCNameDeviceHDD || *pvc.Spec.StorageClassName == OpenLocalSCNameMountPointHDD || *pvc.Spec.StorageClassName == YodaSCNameMountPointHDD || *pvc.Spec.StorageClassName == YodaSCNameDeviceHDD {
			volume := Volume{
				Size: localutils.GetPVCRequested(&pvc),
				Kind: "HDD",
			}
			volumes.Volumes = append(volumes.Volumes, volume)
		}
	}

	for ordinal := 0; ordinal < int(*set.Spec.Replicas); ordinal++ {
		pod, _ := controller.GetPodFromTemplate(&set.Spec.Template, set, nil)
		pod.ObjectMeta.Name = fmt.Sprintf("statefulset-%s-%d", set.Name, ordinal)
		pod.ObjectMeta.Namespace = set.Namespace
		pod = MakePodValid(pod)
		pod = AddWorkloadInfoToPod(pod, simontype.WorkloadKindStatefulSet, set.Name, pod.Namespace)

		// Storage
		b, err := json.Marshal(volumes)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		metav1.SetMetaDataAnnotation(&pod.ObjectMeta, simontype.AnnoPodLocalStorage, string(b))

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

// NodeShouldRunDaemonPod determines whether a node should run a pod generated by daemonset according to scheduling rules
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
	pod.ObjectMeta.Name = fmt.Sprintf("daemonset-%s-%s", ds.Name, nodeName)
	pod.ObjectMeta.Namespace = ds.Namespace
	pod = MakePodValid(pod)
	pod = AddWorkloadInfoToPod(pod, simontype.WorkloadKindDaemonSet, ds.Name, pod.Namespace)
	pod.Spec.NodeName = nodeName
	return pod
}

func MakeValidPodByPod(pod *corev1.Pod) *corev1.Pod {
	return MakePodValid(pod)
}

// MakePodValid make pod valid, so we can handle it
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
	// Probe may cause that pod can not pass the ValidatePod test
	for i := range newPod.Spec.Containers {
		newPod.Spec.Containers[i].LivenessProbe = nil
		newPod.Spec.Containers[i].ReadinessProbe = nil
		newPod.Spec.Containers[i].StartupProbe = nil
	}
	// Add pod provisioner annotation
	if newPod.ObjectMeta.Annotations == nil {
		newPod.ObjectMeta.Annotations = map[string]string{}
	}
	// todo: handle pvc
	if !ValidatePod(newPod) {
		os.Exit(1)
	}

	return newPod
}

// AddWorkloadInfoToPod add annotation in pod for simulating later
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
	node.ObjectMeta.UID = uuid.NewUUID()
	if !ValidateNode(node) {
		os.Exit(1)
	}
	return node
}

// ValidatePod check if pod is valid
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

// ValidateNode check if node is valid
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

// CountPodOnTheNode get pods count by node name
func CountPodOnTheNode(podList *corev1.PodList, nodeName string) (count int64) {
	count = 0
	for _, pod := range podList.Items {
		if pod.Spec.NodeName == nodeName {
			count++
		}
	}
	return
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
	if strings.Contains(queue.nodes[i], simontype.NewNodeNamePrefix) && strings.Contains(queue.nodes[j], simontype.NewNodeNamePrefix) {
		return queue.nodes[i] < queue.nodes[j]
	}

	if !strings.Contains(queue.nodes[i], simontype.NewNodeNamePrefix) && !strings.Contains(queue.nodes[j], simontype.NewNodeNamePrefix) {
		return queue.nodes[i] < queue.nodes[j]
	}

	if !strings.Contains(queue.nodes[i], simontype.NewNodeNamePrefix) {
		return true
	}

	if !strings.Contains(queue.nodes[j], simontype.NewNodeNamePrefix) {
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

func GetTotalNumberOfPodsWithoutNodeName(pods []*corev1.Pod) int64 {
	var podsWithoutNodeNameCount int64 = 0
	for _, item := range pods {
		if item.Spec.NodeName == "" {
			podsWithoutNodeNameCount++
		}
	}
	return podsWithoutNodeNameCount
}
