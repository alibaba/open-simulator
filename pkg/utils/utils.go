package utils

import (
	"encoding/json"
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
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	resourcehelper "k8s.io/kubectl/pkg/util/resource"
	api "k8s.io/kubernetes/pkg/apis/core"
	apiv1 "k8s.io/kubernetes/pkg/apis/core/v1"
	"k8s.io/kubernetes/pkg/apis/core/validation"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/daemon"
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

// DecodeYamlContent captures the yml or yaml file, and decodes it
func DecodeYamlContent(yamlRes []byte) ([]runtime.Object, error) {
	objects := make([]runtime.Object, 0)
	yamls := releaseutil.SplitManifests(string(yamlRes))
	for _, yaml := range yamls {
		decode := scheme.Codecs.UniversalDeserializer().Decode
		obj, _, err := decode([]byte(yaml), nil, nil)
		if err != nil {
			return nil, err
		}

		objects = append(objects, obj)
	}

	return objects, nil
}

func ReadYamlFile(path string) []byte {
	fileExtension := filepath.Ext(path)
	if fileExtension != ".yaml" && fileExtension != ".yml" {
		return nil
	}
	yamlFile, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Printf("Error while read file %s: %s\n", path, err.Error())
		os.Exit(1)
	}
	return yamlFile
}

func ReadJsonFile(path string) []byte {
	pathExtension := filepath.Ext(path)
	if pathExtension != ".json" {
		return nil
	}
	jsonFile, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Printf("Error while read file %s: %s\n", path, err.Error())
		os.Exit(1)
	}

	return jsonFile
}

// GetYamlContentFromDirectory returns the yaml content and ignores other content
func GetYamlContentFromDirectory(dir string) ([]string, error) {
	var ymlStr []string
	filePaths, err := ParseFilePath(dir)
	if err != nil {
		return ymlStr, fmt.Errorf("failed to parse the directory: %v", err)
	}
	for _, filePath := range filePaths {
		if yml := ReadYamlFile(filePath); yml != nil {
			ymlStr = append(ymlStr, string(yml))
		}
	}

	return ymlStr, nil
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

func MakeValidPodByJob(job *batchv1.Job) []*corev1.Pod {
	var pods []*corev1.Pod
	if job.Spec.Completions == nil {
		var completions int32 = 1
		job.Spec.Completions = &completions
	}

	for ordinal := 0; ordinal < int(*job.Spec.Completions); ordinal++ {
		pod, _ := controller.GetPodFromTemplate(&job.Spec.Template, job, nil)
		pod.ObjectMeta.Name = fmt.Sprintf("job-%s-%d", job.GetName(), ordinal)
		pod.ObjectMeta.Namespace = job.GetNamespace()
		pod = MakePodValid(pod)
		pod = AddWorkloadInfoToPod(pod, simontype.WorkloadKindDeployment, pod.Name, pod.Namespace)
		pods = append(pods, pod)
	}

	return pods
}

func MakeValidPodByCronJob(cronjob *batchv1beta1.CronJob) []*corev1.Pod {
	job := new(batchv1.Job)
	job.ObjectMeta.Name = cronjob.Name
	job.ObjectMeta.Namespace = cronjob.Namespace
	job.Spec = cronjob.Spec.JobTemplate.Spec

	pods := MakeValidPodByJob(job)
	return pods
}

type NodeStorage struct {
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

func GetNodeStorage(node *corev1.Node) *NodeStorage {
	nodeStorageStr, exist := node.Annotations[simontype.AnnoNodeLocalStorage]
	if !exist {
		return nil
	}

	nodeStorage := new(NodeStorage)
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
		shouldRun := NodeShouldRunPod(node, pod)
		if shouldRun {
			pods = append(pods, pod)
		}
	}
	return pods
}

// NodeShouldRunPod determines whether a node should run a pod according to scheduling rules
func NodeShouldRunPod(node *corev1.Node, pod *corev1.Pod) bool {
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
	pod.Spec.Affinity = SetDaemonSetPodNodeNameByNodeAffinity(pod.Spec.Affinity, nodeName)
	pod = MakePodValid(pod)
	pod = AddWorkloadInfoToPod(pod, simontype.WorkloadKindDaemonSet, ds.Name, pod.Namespace)
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
			newPod.Spec.InitContainers[i].VolumeMounts = nil
			newPod.Spec.InitContainers[i].Env = nil
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
			newPod.Spec.Containers[i].Env = nil
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

	// handle volume
	// notice: open-local volume should not used in deployment, cause it is not nas storage.
	if newPod.Spec.Volumes != nil {
		for i := range newPod.Spec.Volumes {
			if newPod.Spec.Volumes[i].PersistentVolumeClaim != nil {
				newPod.Spec.Volumes[i].HostPath = new(corev1.HostPathVolumeSource)
				newPod.Spec.Volumes[i].HostPath.Path = "/tmp"
				newPod.Spec.Volumes[i].PersistentVolumeClaim = nil
			}
		}
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

func GetPodsTotalRequestsAndLimitsByNodeName(pods []corev1.Pod, nodeName string) (map[corev1.ResourceName]resource.Quantity, map[corev1.ResourceName]resource.Quantity) {
	reqs, limits := make(map[corev1.ResourceName]resource.Quantity), make(map[corev1.ResourceName]resource.Quantity)
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
	return reqs, limits
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

func MeetResourceRequests(node *corev1.Node, pod *corev1.Pod, daemonSets []*apps.DaemonSet) bool {
	// CPU and Memory
	totalResource := map[corev1.ResourceName]*resource.Quantity{
		corev1.ResourceCPU:    resource.NewQuantity(0, resource.DecimalSI),
		corev1.ResourceMemory: resource.NewQuantity(0, resource.DecimalSI),
	}

	for _, item := range daemonSets {
		newItem := item
		daemonPod := NewDaemonPod(newItem, simontype.NewNodeNamePrefix)
		if NodeShouldRunPod(node, daemonPod) {
			for _, container := range daemonPod.Spec.Containers {
				totalResource[corev1.ResourceCPU].Add(*container.Resources.Requests.Cpu())
				totalResource[corev1.ResourceMemory].Add(*container.Resources.Requests.Memory())
			}
		}
	}
	for _, container := range pod.Spec.Containers {
		totalResource[corev1.ResourceCPU].Add(*container.Resources.Requests.Cpu())
		totalResource[corev1.ResourceMemory].Add(*container.Resources.Requests.Memory())
	}

	if totalResource[corev1.ResourceCPU].Cmp(*node.Status.Allocatable.Cpu()) == 1 ||
		totalResource[corev1.ResourceMemory].Cmp(*node.Status.Allocatable.Memory()) == 1 {
		return false
	}

	// Local Storage
	nodeStorage := GetNodeStorage(node)
	var nodeVGMax int64 = 0
	for _, vg := range nodeStorage.VGs {
		if vg.Capacity > int64(nodeVGMax) {
			nodeVGMax = vg.Capacity
		}
	}
	lvmPVCs, _ := GetPodLocalPVCs(pod)
	var pvcSum int64 = 0
	for _, pvc := range lvmPVCs {
		pvcSum += localutils.GetPVCRequested(pvc)
	}

	return pvcSum <= nodeVGMax
}

func CreateKubeClient(kubeconfig string) (*clientset.Clientset, error) {
	if len(kubeconfig) == 0 {
		return nil, nil
	}

	var err error
	var cfg *restclient.Config
	master, err := GetMasterFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse kubeclient file: %v ", err)
	}

	cfg, err = clientcmd.BuildConfigFromFlags(master, kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("Unable to build config: %v ", err)
	}

	kubeClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return kubeClient, nil
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

func SetDaemonSetPodNodeNameByNodeAffinity(affinity *corev1.Affinity, nodename string) *corev1.Affinity {
	nodeSelReq := corev1.NodeSelectorRequirement{
		Key:      api.ObjectNameField,
		Operator: corev1.NodeSelectorOpIn,
		Values:   []string{nodename},
	}

	nodeSelector := &corev1.NodeSelector{
		NodeSelectorTerms: []corev1.NodeSelectorTerm{
			{
				MatchFields: []corev1.NodeSelectorRequirement{nodeSelReq},
			},
		},
	}

	if affinity == nil {
		return &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: nodeSelector,
			},
		}
	}

	if affinity.NodeAffinity == nil {
		affinity.NodeAffinity = &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: nodeSelector,
		}
		return affinity
	}

	if affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = nodeSelector
		return affinity
	}

	// Replace node selector with the new one.

	nodeSelectorTerms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	for i, item := range nodeSelectorTerms {
		item.MatchFields = []corev1.NodeSelectorRequirement{nodeSelReq}
		nodeSelectorTerms[i] = item
	}
	affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = nodeSelectorTerms

	return affinity
}
