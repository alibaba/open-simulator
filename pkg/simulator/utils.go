package simulator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
	apps "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	v1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	externalclientset "k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/events"
	configv1alpha1 "k8s.io/component-base/config/v1alpha1"
	"k8s.io/component-base/logs"
	kubeschedulerconfigv1beta1 "k8s.io/kube-scheduler/config/v1beta1"
	"k8s.io/kubernetes/cmd/kube-scheduler/app/config"
	schedconfig "k8s.io/kubernetes/cmd/kube-scheduler/app/config"
	schedoptions "k8s.io/kubernetes/cmd/kube-scheduler/app/options"
	kubeschedulerconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	kubeschedulerscheme "k8s.io/kubernetes/pkg/scheduler/apis/config/scheme"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/profile"

	simontype "github.com/alibaba/open-simulator/pkg/type"
	"github.com/alibaba/open-simulator/pkg/utils"
)

// GenerateValidPodsFromAppResources generate valid pods from resources
func GenerateValidPodsFromAppResources(client externalclientset.Interface, appname string, resources ResourceTypes) ([]*corev1.Pod, error) {
	pods := make([]*corev1.Pod, 0)
	validPods, err := GetValidPodExcludeDaemonSet(resources)
	if err != nil {
		return nil, err
	}
	pods = append(pods, validPods...)

	// DaemonSet will match with specific nodes so it needs to be handled separately
	var nodes []*corev1.Node

	// get all nodes
	nodeItems, err := client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %v ", err)
	}
	for _, item := range nodeItems.Items {
		newItem := item
		nodes = append(nodes, &newItem)
	}

	// get all pods from daemonset
	for _, item := range resources.DaemonSets {
		newItem := item
		validPods, err := utils.MakeValidPodsByDaemonset(newItem, nodes)
		if err != nil {
			return nil, err
		}
		pods = append(pods, validPods...)
	}

	// set label
	for _, pod := range pods {
		metav1.SetMetaDataLabel(&pod.ObjectMeta, simontype.LabelAppName, appname)
	}

	return pods, nil
}

// GetValidPodExcludeDaemonSet gets valid pod by resources exclude DaemonSet that needs to be handled specially
const RoutinePodNum = 100

func GetValidPodExcludeDaemonSet(resources ResourceTypes) ([]*corev1.Pod, error) {
	pods := make([]*corev1.Pod, 0)
	var wg sync.WaitGroup
	var rstErr error = nil
	mu := &sync.Mutex{}
	errMu := &sync.Mutex{}
	if len(resources.Pods) > 0 {
		handlePods := func(tmpPods []*corev1.Pod) {
			defer wg.Done()
			tmpValidPods := make([]*corev1.Pod, 0)
			for _, pod := range tmpPods {
				validPod, err := utils.MakeValidPodByPod(pod)
				if err != nil {
					errMu.Lock()
					rstErr = err
					errMu.Unlock()
					return
				}
				tmpValidPods = append(tmpValidPods, validPod)
			}
			mu.Lock()
			pods = append(pods, tmpValidPods...)
			mu.Unlock()
		}
		var start, end int
		routineNum := len(resources.Pods) / RoutinePodNum
		for i := 0; i <= routineNum; i++ {
			start = i * RoutinePodNum
			if i != routineNum {
				end = (i + 1) * RoutinePodNum
			} else {
				end = len(resources.Pods)
			}
			wg.Add(1)
			go handlePods(resources.Pods[start:end])
		}
	}

	if len(resources.Deployments) > 0 {
		handleDeployments := func() {
			defer wg.Done()
			tmpValidPods := make([]*corev1.Pod, 0)
			for _, deploy := range resources.Deployments {
				validPods, err := utils.MakeValidPodsByDeployment(deploy)
				if err != nil {
					errMu.Lock()
					rstErr = fmt.Errorf("fail to make valid pods by deployment %s: %s\n", deploy.Name, err.Error())
					errMu.Unlock()
					return
				}
				tmpValidPods = append(tmpValidPods, validPods...)
			}
			mu.Lock()
			pods = append(pods, tmpValidPods...)
			mu.Unlock()
		}
		wg.Add(1)
		go handleDeployments()
	}

	if len(resources.ReplicaSets) > 0 {
		handleReplicaSets := func() {
			defer wg.Done()
			tmpValidPods := make([]*corev1.Pod, 0)
			for _, rs := range resources.ReplicaSets {
				validPods, err := utils.MakeValidPodsByReplicaSet(rs)
				if err != nil {
					errMu.Lock()
					rstErr = fmt.Errorf("fail to make valid pods by replicaset %s: %s\n", rs.Name, err.Error())
					errMu.Unlock()
					return
				}
				tmpValidPods = append(tmpValidPods, validPods...)
			}
			mu.Lock()
			pods = append(pods, tmpValidPods...)
			mu.Unlock()
		}
		wg.Add(1)
		go handleReplicaSets()
	}

	if len(resources.StatefulSets) > 0 {
		handleStatefulSets := func() {
			defer wg.Done()
			tmpValidPods := make([]*corev1.Pod, 0)
			for _, sts := range resources.StatefulSets {
				validPods, err := utils.MakeValidPodsByStatefulSet(sts)
				if err != nil {
					errMu.Lock()
					rstErr = fmt.Errorf("fail to make valid pods by statefulset %s: %s\n", sts.Name, err.Error())
					errMu.Unlock()
					return
				}
				tmpValidPods = append(tmpValidPods, validPods...)
			}
			mu.Lock()
			pods = append(pods, tmpValidPods...)
			mu.Unlock()
		}
		wg.Add(1)
		go handleStatefulSets()
	}

	if len(resources.Jobs) > 0 {
		handleJobs := func() {
			defer wg.Done()
			tmpValidPods := make([]*corev1.Pod, 0)
			for _, job := range resources.Jobs {
				validPods, err := utils.MakeValidPodByJob(job)
				if err != nil {
					errMu.Lock()
					rstErr = fmt.Errorf("fail to make valid pods by job %s: %s\n", job.Name, err.Error())
					errMu.Unlock()
					return
				}
				tmpValidPods = append(tmpValidPods, validPods...)
			}
			mu.Lock()
			pods = append(pods, tmpValidPods...)
			mu.Unlock()
		}
		wg.Add(1)
		go handleJobs()
	}

	if len(resources.CronJobs) > 0 {
		handleCronJobs := func() {
			defer wg.Done()
			tmpValidPods := make([]*corev1.Pod, 0)
			for _, cronjob := range resources.CronJobs {
				validPods, err := utils.MakeValidPodByCronJob(cronjob)
				if err != nil {
					errMu.Lock()
					rstErr = fmt.Errorf("fail to make valid pods by cronjob %s: %s\n", cronjob.Name, err.Error())
					errMu.Unlock()
					return
				}
				tmpValidPods = append(tmpValidPods, validPods...)
			}
			mu.Lock()
			pods = append(pods, tmpValidPods...)
			mu.Unlock()
		}
		wg.Add(1)
		go handleCronJobs()
	}

	wg.Wait()

	return pods, rstErr
}

// GetObjectFromYamlContent decodes the yaml content and returns the kubernetes objects
func GetObjectFromYamlContent(ymlStr []string) (ResourceTypes, error) {
	var resources ResourceTypes

	for _, res := range ymlStr {
		objects, err := utils.DecodeYamlContent([]byte(res))
		if err != nil {
			return ResourceTypes{}, err
		}
		for _, obj := range objects {
			switch o := obj.(type) {
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
			case *apps.ReplicaSet:
				resources.ReplicaSets = append(resources.ReplicaSets, o)
			case *batchv1.Job:
				resources.Jobs = append(resources.Jobs, o)
			case *batchv1beta1.CronJob:
				resources.CronJobs = append(resources.CronJobs, o)
			case *v1.StorageClass:
				resources.StorageClasss = append(resources.StorageClasss, o)
			case *v1beta1.PodDisruptionBudget:
				resources.PodDisruptionBudgets = append(resources.PodDisruptionBudgets, o)
			default:
				log.Debugf("unknown type(%T): %s\n", o, ymlStr)
				continue
			}
		}
	}

	return resources, nil
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

// GetAndSetSchedulerConfig gets scheduler CompletedConfig and sets the list of scheduler bind plugins to Simon.
func GetAndSetSchedulerConfig(schedulerConfig string) (*config.CompletedConfig, error) {
	versionedCfg := kubeschedulerconfigv1beta1.KubeSchedulerConfiguration{}
	versionedCfg.DebuggingConfiguration = *configv1alpha1.NewRecommendedDebuggingConfiguration()
	kubeschedulerscheme.Scheme.Default(&versionedCfg)
	kcfg := kubeschedulerconfig.KubeSchedulerConfiguration{}
	if err := kubeschedulerscheme.Scheme.Convert(&versionedCfg, &kcfg, nil); err != nil {
		return nil, err
	}
	if len(kcfg.Profiles) == 0 {
		kcfg.Profiles = []kubeschedulerconfig.KubeSchedulerProfile{
			{},
		}
	}
	kcfg.Profiles[0].SchedulerName = corev1.DefaultSchedulerName
	if kcfg.Profiles[0].Plugins == nil {
		kcfg.Profiles[0].Plugins = &kubeschedulerconfig.Plugins{}
	}
	kcfg.Profiles[0].Plugins.Score = &kubeschedulerconfig.PluginSet{
		Enabled: []kubeschedulerconfig.Plugin{
			{
				Name: simontype.SimonPluginName,
			},
			{
				Name: simontype.OpenLocalPluginName,
			},
			{
				Name: simontype.OpenGpuSharePluginName,
			},
		},
	}
	kcfg.Profiles[0].Plugins.Filter = &kubeschedulerconfig.PluginSet{
		Enabled: []kubeschedulerconfig.Plugin{
			{
				Name: simontype.OpenLocalPluginName,
			},
			{
				Name: simontype.OpenGpuSharePluginName,
			},
		},
	}
	kcfg.Profiles[0].Plugins.Reserve = &kubeschedulerconfig.PluginSet{
		Enabled: []kubeschedulerconfig.Plugin{
			{
				Name: simontype.OpenGpuSharePluginName,
			},
		},
	}
	kcfg.Profiles[0].Plugins.Bind = &kubeschedulerconfig.PluginSet{
		Enabled: []kubeschedulerconfig.Plugin{
			{
				Name: simontype.OpenLocalPluginName,
			},
			{
				Name: simontype.OpenGpuSharePluginName,
			},
			{
				Name: simontype.SimonPluginName,
			},
		},
		Disabled: []kubeschedulerconfig.Plugin{
			{
				Name: defaultbinder.Name,
			},
		},
	}
	// set percentageOfNodesToScore value to 100
	kcfg.PercentageOfNodesToScore = 100
	opts := &schedoptions.Options{
		ComponentConfig: kcfg,
		ConfigFile:      schedulerConfig,
		Logs:            logs.NewOptions(),
	}
	cc, err := InitKubeSchedulerConfiguration(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to init kube scheduler configuration: %v ", err)
	}
	return cc, nil
}

// MatchAndSetLocalStorageAnnotationOnNode add storage information configured by json file, belonging to the node that matches
// the json file name, to annotation of this node
func MatchAndSetLocalStorageAnnotationOnNode(nodes []*corev1.Node, dir string) {
	storageInfo := make(map[string]string)

	filePaths, _ := utils.ParseFilePath(dir)
	for _, filePath := range filePaths {
		if json := utils.ReadJsonFile(filePath); json != nil {
			name := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
			storageInfo[name] = string(json)
		}
	}

	for _, node := range nodes {
		if info, exist := storageInfo[node.Name]; exist {
			metav1.SetMetaDataAnnotation(&node.ObjectMeta, simontype.AnnoNodeLocalStorage, info)
		}
	}
}
