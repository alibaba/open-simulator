package utils

import (
	"crypto/sha256"
	"encoding/hex"
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
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/uuid"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	resourcehelper "k8s.io/kubectl/pkg/util/resource"
	api "k8s.io/kubernetes/pkg/apis/core"
	apiv1 "k8s.io/kubernetes/pkg/apis/core/v1"
	"k8s.io/kubernetes/pkg/apis/core/validation"
	"k8s.io/kubernetes/pkg/controller/daemon"
)

// ParseFilePath converts recursively directory path to a slice of file paths
func ParseFilePath(path string) (filePaths []string, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse path(%s): %v ", path, err)
	}

	switch mode := fi.Mode(); {
	case mode.IsDir():
		files, err := ioutil.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory(%s): %v ", path, err)
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
	default:
		return nil, fmt.Errorf("invalid path: %s ", path)
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
			return nil, fmt.Errorf("failed to decode yaml content: \n%s\n%v", yaml, err)
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
		return ymlStr, err
	}
	for _, filePath := range filePaths {
		if yml := ReadYamlFile(filePath); yml != nil {
			ymlStr = append(ymlStr, string(yml))
		}
	}

	return ymlStr, nil
}

func MakeValidPodsByDeployment(deploy *appsv1.Deployment) ([]*corev1.Pod, error) {
	deploy.UID = uuid.NewUUID()
	return MakeValidPodsByReplicaSet(generateReplicaSetFromDeployment(deploy))
}

func MakeValidPodsByReplicaSet(rs *appsv1.ReplicaSet) ([]*corev1.Pod, error) {
	var pods []*corev1.Pod
	if rs.UID == "" {
		rs.UID = uuid.NewUUID()
	}
	if rs.Spec.Replicas == nil {
		var replica int32 = 1
		rs.Spec.Replicas = &replica
	}
	for ordinal := 0; ordinal < int(*rs.Spec.Replicas); ordinal++ {
		pod := &corev1.Pod{
			ObjectMeta: SetObjectMetaFromObject(rs, true),
			Spec:       rs.Spec.Template.Spec,
		}
		validPod, err := MakeValidPod(pod)
		if err != nil {
			return nil, err
		}
		validPod = AddWorkloadInfoToPod(validPod, simontype.ReplicaSet, rs.Name, rs.Namespace)
		pods = append(pods, validPod)
	}
	return pods, nil
}

func MakeValidPodsByReplicationController(rc *corev1.ReplicationController) ([]*corev1.Pod, error) {
	var pods []*corev1.Pod
	rc.UID = uuid.NewUUID()
	if rc.Spec.Replicas == nil {
		var replica int32 = 1
		rc.Spec.Replicas = &replica
	}

	for ordinal := 0; ordinal < int(*rc.Spec.Replicas); ordinal++ {
		pod := &corev1.Pod{
			ObjectMeta: SetObjectMetaFromObject(rc, true),
			Spec:       rc.Spec.Template.Spec,
		}
		validPod, err := MakeValidPod(pod)
		if err != nil {
			return nil, err
		}
		validPod = AddWorkloadInfoToPod(validPod, simontype.ReplicationController, rc.Name, rc.Namespace)
		pods = append(pods, validPod)
	}
	return pods, nil
}

func generateReplicaSetFromDeployment(deploy *appsv1.Deployment) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{
		TypeMeta:   metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: simontype.ReplicaSet},
		ObjectMeta: SetObjectMetaFromObject(deploy, false),
		Spec: appsv1.ReplicaSetSpec{
			Selector: deploy.Spec.Selector,
			Replicas: deploy.Spec.Replicas,
			Template: deploy.Spec.Template,
		},
	}
}

func MakeValidPodByCronJob(cronjob *batchv1beta1.CronJob) ([]*corev1.Pod, error) {
	cronjob.UID = uuid.NewUUID()
	return MakeValidPodByJob(generateJobFromCronJob(cronjob))
}

func MakeValidPodByJob(job *batchv1.Job) ([]*corev1.Pod, error) {
	var pods []*corev1.Pod
	if job.UID == "" {
		job.UID = uuid.NewUUID()
	}
	job.UID = uuid.NewUUID()
	if job.Spec.Completions == nil {
		var completions int32 = 1
		job.Spec.Completions = &completions
	}

	for ordinal := 0; ordinal < int(*job.Spec.Completions); ordinal++ {
		pod := &corev1.Pod{
			ObjectMeta: SetObjectMetaFromObject(job, true),
			Spec:       job.Spec.Template.Spec,
		}
		validPod, err := MakeValidPod(pod)
		if err != nil {
			return nil, err
		}
		validPod = AddWorkloadInfoToPod(validPod, simontype.Job, job.Name, job.Namespace)
		pods = append(pods, validPod)
	}

	return pods, nil
}

func generateJobFromCronJob(cronJob *batchv1beta1.CronJob) *batchv1.Job {
	annotations := make(map[string]string)
	annotations["cronjob.kubernetes.io/instantiate"] = "manual"
	for k, v := range cronJob.Spec.JobTemplate.Annotations {
		annotations[k] = v
	}

	return &batchv1.Job{
		TypeMeta:   metav1.TypeMeta{APIVersion: batchv1.SchemeGroupVersion.String(), Kind: simontype.Job},
		ObjectMeta: SetObjectMetaFromObject(cronJob, false),
		Spec:       cronJob.Spec.JobTemplate.Spec,
	}
}

func MakeValidPodsByStatefulSet(ss *appsv1.StatefulSet) ([]*corev1.Pod, error) {
	var pods []*corev1.Pod
	ss.UID = uuid.NewUUID()
	if ss.Spec.Replicas == nil {
		var replica int32 = 1
		ss.Spec.Replicas = &replica
	}

	for ordinal := 0; ordinal < int(*ss.Spec.Replicas); ordinal++ {
		pod := &corev1.Pod{
			ObjectMeta: SetObjectMetaFromObject(ss, true),
			Spec:       ss.Spec.Template.Spec,
		}
		validPod, err := MakeValidPod(pod)
		if err != nil {
			return nil, err
		}
		validPod.Name = fmt.Sprintf("%s-%d", ss.Name, ordinal)
		validPod = AddWorkloadInfoToPod(validPod, simontype.StatefulSet, ss.Name, ss.Namespace)

		pods = append(pods, validPod)
	}

	if err := SetStorageAnnotationOnPods(pods, ss.Spec.VolumeClaimTemplates, ss.Name); err != nil {
		return nil, err
	}

	return pods, nil
}

func SetStorageAnnotationOnPods(pods []*corev1.Pod, volumeClaimTemplates []corev1.PersistentVolumeClaim, stsName string) error {
	var volumes VolumeRequest
	volumes.Volumes = make([]Volume, 0)
	for _, pvc := range volumeClaimTemplates {
		if pvc.Spec.StorageClassName != nil {
			if *pvc.Spec.StorageClassName == OpenLocalSCNameLVM || *pvc.Spec.StorageClassName == YodaSCNameLVM {
				volume := Volume{
					Size:             localutils.GetPVCRequested(&pvc),
					Kind:             "LVM",
					StorageClassName: *pvc.Spec.StorageClassName,
				}
				volumes.Volumes = append(volumes.Volumes, volume)
			} else if *pvc.Spec.StorageClassName == OpenLocalSCNameDeviceSSD || *pvc.Spec.StorageClassName == OpenLocalSCNameMountPointSSD || *pvc.Spec.StorageClassName == YodaSCNameMountPointSSD || *pvc.Spec.StorageClassName == YodaSCNameDeviceSSD {
				volume := Volume{
					Size:             localutils.GetPVCRequested(&pvc),
					Kind:             "SSD",
					StorageClassName: *pvc.Spec.StorageClassName,
				}
				volumes.Volumes = append(volumes.Volumes, volume)
			} else if *pvc.Spec.StorageClassName == OpenLocalSCNameDeviceHDD || *pvc.Spec.StorageClassName == OpenLocalSCNameMountPointHDD || *pvc.Spec.StorageClassName == YodaSCNameMountPointHDD || *pvc.Spec.StorageClassName == YodaSCNameDeviceHDD {
				volume := Volume{
					Size:             localutils.GetPVCRequested(&pvc),
					Kind:             "HDD",
					StorageClassName: *pvc.Spec.StorageClassName,
				}
				volumes.Volumes = append(volumes.Volumes, volume)
			} else {
				log.Errorf("unsupported storage class: %s", *pvc.Spec.StorageClassName)
			}
		} else {
			log.Errorf("empty storageClassName in volumeTemplate of statefulset %s is not supported", stsName)
		}
	}

	for _, pod := range pods {
		b, err := json.Marshal(volumes)
		if err != nil {
			return err
		}
		metav1.SetMetaDataAnnotation(&pod.ObjectMeta, simontype.AnnoPodLocalStorage, string(b))
	}

	return nil
}

func SetObjectMetaFromObject(owner metav1.Object, genPod bool) metav1.ObjectMeta {
	var controllerKind schema.GroupVersionKind
	switch owner.(type) {
	case *appsv1.Deployment:
		controllerKind = appsv1.SchemeGroupVersion.WithKind(simontype.Deployment)
	case *appsv1.ReplicaSet:
		controllerKind = appsv1.SchemeGroupVersion.WithKind(simontype.ReplicaSet)
	case *appsv1.StatefulSet:
		controllerKind = appsv1.SchemeGroupVersion.WithKind(simontype.StatefulSet)
	case *appsv1.DaemonSet:
		controllerKind = appsv1.SchemeGroupVersion.WithKind(simontype.DaemonSet)
	case *corev1.ReplicationController:
		controllerKind = corev1.SchemeGroupVersion.WithKind(simontype.ReplicationController)
	case *batchv1beta1.CronJob:
		controllerKind = batchv1beta1.SchemeGroupVersion.WithKind(simontype.CronJob)
	case *batchv1.Job:
		controllerKind = batchv1.SchemeGroupVersion.WithKind(simontype.Job)
	}
	return metav1.ObjectMeta{
		Name:         owner.GetName() + simontype.SeparateSymbol + GetSHA256HashCode([]byte(rand.String(10)), GetObjectHashCodeDigit(genPod)),
		Namespace:    owner.GetNamespace(),
		UID:          uuid.NewUUID(),
		Annotations:  owner.GetAnnotations(),
		GenerateName: owner.GetName(),
		Labels:       owner.GetLabels(),
		OwnerReferences: []metav1.OwnerReference{
			*metav1.NewControllerRef(owner, controllerKind),
		},
	}
}

func GetObjectHashCodeDigit(isPod bool) int {
	if isPod {
		return simontype.PodHashCodeDigit
	}
	return simontype.WorkLoadHashCodeDigit
}

func MakeValidPodsByDaemonset(ds *appsv1.DaemonSet, nodes []*corev1.Node) ([]*corev1.Pod, error) {
	var pods []*corev1.Pod
	ds.UID = uuid.NewUUID()
	for _, node := range nodes {
		pod, err := NewDaemonPod(ds, node.Name)
		if err != nil {
			return nil, err
		}
		shouldRun := NodeShouldRunPod(node, pod)
		if shouldRun {
			pods = append(pods, pod)
		}
	}
	return pods, nil
}

func NewDaemonPod(ds *appsv1.DaemonSet, nodeName string) (*corev1.Pod, error) {
	pod := &corev1.Pod{
		ObjectMeta: SetObjectMetaFromObject(ds, true),
		Spec:       ds.Spec.Template.Spec,
	}
	pod.Spec.Affinity = SetDaemonSetPodNodeNameByNodeAffinity(pod.Spec.Affinity, nodeName)
	validPod, err := MakeValidPod(pod)
	if err != nil {
		return nil, err
	}
	validPod = AddWorkloadInfoToPod(validPod, simontype.DaemonSet, ds.Name, ds.Namespace)

	return validPod, nil
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

func MakeValidPodByPod(pod *corev1.Pod) (*corev1.Pod, error) {
	pod.UID = uuid.NewUUID()
	newPod, err := MakeValidPod(pod)
	if err != nil {
		return nil, err
	}
	return newPod, nil
}

// MakeValidPod make pod valid, so we can handle it
func MakeValidPod(oldPod *corev1.Pod) (*corev1.Pod, error) {
	newPod := oldPod.DeepCopy()

	if newPod.Labels == nil {
		newPod.Labels = make(map[string]string)
	}

	// ObjectMeta
	if newPod.ObjectMeta.Namespace == "" {
		newPod.ObjectMeta.Namespace = corev1.NamespaceDefault
	}
	if newPod.ObjectMeta.Annotations == nil {
		newPod.ObjectMeta.Annotations = map[string]string{}
	}

	// Spec
	if newPod.Spec.DNSPolicy == "" {
		newPod.Spec.DNSPolicy = corev1.DNSClusterFirst
	}
	if newPod.Spec.RestartPolicy == "" {
		newPod.Spec.RestartPolicy = corev1.RestartPolicyAlways
	}
	if newPod.Spec.SchedulerName == "" {
		newPod.Spec.SchedulerName = simontype.DefaultSchedulerName
	}
	newPod.Spec.ImagePullSecrets = nil

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
			// Probe
			newPod.Spec.Containers[i].LivenessProbe = nil
			newPod.Spec.Containers[i].ReadinessProbe = nil
			newPod.Spec.Containers[i].StartupProbe = nil
		}
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
	if err := ValidatePod(newPod); err != nil {
		return nil, err
	}

	return newPod, nil
}

// AddWorkloadInfoToPod add annotation in pod for simulating later
func AddWorkloadInfoToPod(pod *corev1.Pod, kind string, name string, namespace string) *corev1.Pod {
	pod.ObjectMeta.Annotations[simontype.AnnoWorkloadKind] = kind
	pod.ObjectMeta.Annotations[simontype.AnnoWorkloadName] = name
	pod.ObjectMeta.Annotations[simontype.AnnoWorkloadNamespace] = namespace
	return pod
}

func MakeValidNodeByNode(node *corev1.Node, nodename string) (*corev1.Node, error) {
	node.ObjectMeta.Name = nodename
	node.ObjectMeta.UID = uuid.NewUUID()
	if node.ObjectMeta.Labels == nil {
		node.ObjectMeta.Labels = map[string]string{}
	}
	node.ObjectMeta.Labels[corev1.LabelHostname] = nodename
	if node.ObjectMeta.Annotations == nil {
		node.ObjectMeta.Annotations = map[string]string{}
	}
	if err := ValidateNode(node); err != nil {
		return nil, err
	}
	return node, nil
}

// ValidatePod check if pod is valid
func ValidatePod(pod *corev1.Pod) error {
	internalPod := &api.Pod{}
	if err := apiv1.Convert_v1_Pod_To_core_Pod(pod, internalPod, nil); err != nil {
		return fmt.Errorf("unable to convert to internal version: %#v ", err)
	}
	if errs := validation.ValidatePodCreate(internalPod, validation.PodValidationOptions{}); len(errs) > 0 {
		var errStrs []string
		for _, err := range errs {
			errStrs = append(errStrs, fmt.Sprintf("%v", err))
		}
		return fmt.Errorf("invalid pod: %#v ", strings.Join(errStrs, "\n"))
	}
	return nil
}

func GetSHA256HashCode(message []byte, num int) string {
	hash := sha256.New()
	hash.Write(message)
	hashCode := hex.EncodeToString(hash.Sum(nil))
	return hashCode[:num]
}

type NodeStorage struct {
	VGs     []localcache.SharedResource    `json:"vgs"`
	Devices []localcache.ExclusiveResource `json:"devices"`
}

type Volume struct {
	Size int64 `json:"size,string"`
	// Kind 可以是 LVM 或 HDD 或 SSD
	// HDD 和 SSD 均指代独占盘
	Kind             string `json:"kind"`
	StorageClassName string `json:"scName"`
}

type VolumeRequest struct {
	Volumes []Volume `json:"volumes"`
}

func GetNodeStorage(node *corev1.Node) (*NodeStorage, error) {
	nodeStorageStr, exist := node.Annotations[simontype.AnnoNodeLocalStorage]
	if !exist {
		return nil, nil
	}

	nodeStorage := new(NodeStorage)
	if err := ffjson.Unmarshal([]byte(nodeStorageStr), nodeStorage); err != nil {
		return nil, fmt.Errorf("failed to unmarshal info of node %s: %s ", node.Name, err.Error())
	}

	return nodeStorage, nil
}

func GetNodeCache(node *corev1.Node) (*localcache.NodeCache, error) {
	nodeStorage, err := GetNodeStorage(node)
	if err != nil {
		return nil, err
	} else if nodeStorage == nil {
		return nil, nil
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

	return nc, nil
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
		if volume.Kind == "LVM" || volume.Kind == "HDD" || volume.Kind == "SSD" {
			scName = volume.StorageClassName
		} else {
			log.Errorf("unsupported volume kind: %s", volume.Kind)
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
		if scName == OpenLocalSCNameLVM || scName == YodaSCNameLVM {
			lvmPVCs = append(lvmPVCs, pvc)
		} else {
			devicePVCs = append(devicePVCs, pvc)
		}
	}

	return lvmPVCs, devicePVCs
}

// ValidateNode check if node is valid
func ValidateNode(node *corev1.Node) error {
	internalNode := &api.Node{}
	if err := apiv1.Convert_v1_Node_To_core_Node(node, internalNode, nil); err != nil {
		return fmt.Errorf("unable to convert to internal version: %#v ", err)
	}
	if errs := validation.ValidateNode(internalNode); len(errs) > 0 {
		var errStrs []string
		for _, err := range errs {
			errStrs = append(errStrs, fmt.Sprintf("%v", err))
		}
		return fmt.Errorf("invalid node: %#v ", strings.Join(errStrs, "\n"))
	}

	return nil
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

func MeetResourceRequests(node *corev1.Node, pod *corev1.Pod, daemonSets []*appsv1.DaemonSet) (bool, error) {
	// CPU and Memory
	totalResource := map[corev1.ResourceName]*resource.Quantity{
		corev1.ResourceCPU:    resource.NewQuantity(0, resource.DecimalSI),
		corev1.ResourceMemory: resource.NewQuantity(0, resource.DecimalSI),
	}

	for _, item := range daemonSets {
		newItem := item
		daemonPod, err := NewDaemonPod(newItem, simontype.NewNodeNamePrefix)
		if err != nil {
			return false, err
		}
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
		return false, nil
	}

	// Local Storage
	nodeStorage, err := GetNodeStorage(node)
	if err != nil {
		return false, err
	} else if nodeStorage == nil {
		return true, nil
	}
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

	return pvcSum <= nodeVGMax, nil
}

func CreateKubeClient(kubeconfig string) (*clientset.Clientset, error) {
	if len(kubeconfig) == 0 {
		return nil, nil
	}

	var err error
	var cfg *restclient.Config
	master, err := GetMasterFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse kubeclient file: %v ", err)
	}

	cfg, err = clientcmd.BuildConfigFromFlags(master, kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("unable to build config: %v ", err)
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
		return "", fmt.Errorf("failed to get master address from kubeconfig")
	}

	if val, ok := config.Clusters[context.Cluster]; ok {
		return val.Server, nil
	}
	return "", fmt.Errorf("failed to get master address from kubeconfig")
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
