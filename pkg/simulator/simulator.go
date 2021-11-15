package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"

	simonplugin "github.com/alibaba/open-simulator/pkg/simulator/plugin"
	simontype "github.com/alibaba/open-simulator/pkg/type"
	"github.com/alibaba/open-simulator/pkg/utils"
	"github.com/olekukonko/tablewriter"
	"github.com/pquerna/ffjson/ffjson"
	log "github.com/sirupsen/logrus"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/informers"
	kubeinformers "k8s.io/client-go/informers"
	externalclientset "k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	resourcehelper "k8s.io/kubectl/pkg/util/resource"
	schedconfig "k8s.io/kubernetes/cmd/kube-scheduler/app/config"
	"k8s.io/kubernetes/pkg/scheduler"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
)

// Simulator is used to simulate a cluster and pods scheduling
type Simulator struct {
	// kube client
	externalclient  externalclientset.Interface
	fakeclient      externalclientset.Interface
	informerFactory informers.SharedInformerFactory

	// scheduler
	scheduler     *scheduler.Scheduler
	schedulerName string

	// stopCh
	simulatorStop chan struct{}

	// context
	ctx        context.Context
	cancelFunc context.CancelFunc

	status Status
}

// Status captures reason why one pod fails to be scheduled
type Status struct {
	stopReason string
	// the value of the numOfRemainingPods is reduced by one After one pod is scheduled
	numOfRemainingPods int64
}

// New generates all components that will be needed to simulate scheduling and returns a complete simulator
func New(externalClient externalclientset.Interface, kubeSchedulerConfig *schedconfig.CompletedConfig) (*Simulator, error) {
	var err error
	ctx, cancel := context.WithCancel(context.Background())

	// Step 1: create fake client
	fakeClient := fakeclientset.NewSimpleClientset()
	sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)

	// Step 2: Create the simulator
	sim := &Simulator{
		externalclient:  externalClient,
		fakeclient:      fakeClient,
		simulatorStop:   make(chan struct{}),
		informerFactory: sharedInformerFactory,
		ctx:             ctx,
		cancelFunc:      cancel,
		schedulerName:   simontype.DefaultSchedulerName,
	}

	// Step 3: add event handler for pods
	sim.informerFactory.Core().V1().Pods().Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				if pod, ok := obj.(*corev1.Pod); ok && pod.Spec.SchedulerName == sim.schedulerName {
					return true
				}
				return false
			},
			Handler: cache.ResourceEventHandlerFuncs{
				// AddFunc: func(obj interface{}) {
				// 	if pod, ok := obj.(*corev1.Pod); ok {
				// 		fmt.Printf("test add pod %s/%s\n", pod.Namespace, pod.Name)
				// 	}
				// },
				UpdateFunc: func(oldObj, newObj interface{}) {
					if pod, ok := newObj.(*corev1.Pod); ok {
						// fmt.Printf("test update pod %s/%s\n", pod.Namespace, pod.Name)
						if err := sim.update(pod, sim.schedulerName); err != nil {
							fmt.Printf("update error: %s\n", err.Error())
							return
						}
					}
				},
			},
		},
	)

	// Step 4: create scheduler for fake cluster
	kubeSchedulerConfig.Client = fakeClient
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(sim.fakeclient, 0)
	storagev1Informers := kubeInformerFactory.Storage().V1()
	scInformer := storagev1Informers.StorageClasses().Informer()
	kubeInformerFactory.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), scInformer.HasSynced)
	bindRegistry := frameworkruntime.Registry{
		simontype.SimonPluginName: func(configuration runtime.Object, f framework.Handle) (framework.Plugin, error) {
			return simonplugin.NewSimonPlugin(simontype.DefaultSchedulerName, sim.fakeclient, configuration, f)
		},
		simontype.OpenLocalPluginName: func(configuration runtime.Object, f framework.Handle) (framework.Plugin, error) {
			return simonplugin.NewLocalPlugin(simontype.DefaultSchedulerName, fakeClient, storagev1Informers, configuration, f)
		},
	}
	sim.scheduler, err = scheduler.New(
		sim.fakeclient,
		sim.informerFactory,
		utils.GetRecorderFactory(kubeSchedulerConfig),
		sim.ctx.Done(),
		scheduler.WithProfiles(kubeSchedulerConfig.ComponentConfig.Profiles...),
		scheduler.WithAlgorithmSource(kubeSchedulerConfig.ComponentConfig.AlgorithmSource),
		scheduler.WithPercentageOfNodesToScore(kubeSchedulerConfig.ComponentConfig.PercentageOfNodesToScore),
		scheduler.WithFrameworkOutOfTreeRegistry(bindRegistry),
		scheduler.WithPodMaxBackoffSeconds(kubeSchedulerConfig.ComponentConfig.PodMaxBackoffSeconds),
		scheduler.WithPodInitialBackoffSeconds(kubeSchedulerConfig.ComponentConfig.PodInitialBackoffSeconds),
		scheduler.WithExtenders(kubeSchedulerConfig.ComponentConfig.Extenders...),
	)
	if err != nil {
		return nil, err
	}

	return sim, nil
}

// RunScheduler
func (sim *Simulator) RunScheduler() {
	// Step 1: start all informers.
	sim.informerFactory.Start(sim.ctx.Done())
	sim.informerFactory.WaitForCacheSync(sim.ctx.Done())

	// Step 2: run scheduler
	go func() {
		sim.scheduler.Run(sim.ctx)
	}()
	// Step 3: wait some time before at least nodes are populated
	// time.Sleep(100 * time.Millisecond)
}

// Run starts to schedule pods
func (sim *Simulator) SchedulePods(pods []*corev1.Pod) (*corev1.Pod, error) {
	// create the simulated pods
	sim.status.numOfRemainingPods = utils.GetTotalNumberOfPodsWithoutNodeName(pods)
	for _, pod := range pods {
		_, err := sim.fakeclient.CoreV1().Pods(pod.Namespace).Create(context.Background(), pod, metav1.CreateOptions{})
		if err != nil {
			return pod, fmt.Errorf("%s %s/%s: %s", simontype.CreateError, pod.Namespace, pod.Name, err.Error())
		}

		// we send value into sim.simulatorStop channel in update() function only,
		// update() is triggered when pod without nodename is handled.
		if pod.Spec.NodeName == "" {
			<-sim.simulatorStop
		}

		if strings.Contains(sim.status.stopReason, "failed") {
			return pod, fmt.Errorf("%s\n", sim.GetStatus())
		}
	}

	return nil, nil
}

// GetStatus return StopReason
func (sim *Simulator) GetStatus() string {
	return sim.status.stopReason
}

// Report print out scheduling result of pods
func (sim *Simulator) Report() {
	// Step 1: report pod info
	fmt.Println("Pod Info")
	podTable := tablewriter.NewWriter(os.Stdout)
	podTable.SetHeader([]string{
		"Node",
		"Pod",
		"CPU Requests",
		"Memory Requests",
		"Volume Requests",
		"App Name",
	})

	nodes, _ := sim.fakeclient.CoreV1().Nodes().List(sim.ctx, metav1.ListOptions{})
	allPods, _ := sim.fakeclient.CoreV1().Pods(corev1.NamespaceAll).List(sim.ctx, metav1.ListOptions{
		// FieldSelector not work
		// issue: https://github.com/kubernetes/client-go/issues/326#issuecomment-412993326
		// FieldSelector: "spec.nodeName=%s" + node.Name,
	})
	for _, node := range nodes.Items {
		allocatable := node.Status.Allocatable
		for _, pod := range allPods.Items {
			if pod.Spec.NodeName != node.Name {
				continue
			}
			req, limit := resourcehelper.PodRequestsAndLimits(&pod)
			cpuReq, _, memoryReq, _ := req[corev1.ResourceCPU], limit[corev1.ResourceCPU], req[corev1.ResourceMemory], limit[corev1.ResourceMemory]
			fractionCpuReq := float64(cpuReq.MilliValue()) / float64(allocatable.Cpu().MilliValue()) * 100
			fractionMemoryReq := float64(memoryReq.Value()) / float64(allocatable.Memory().Value()) * 100

			// app name
			appname := ""
			if str, exist := pod.Labels[simontype.LabelAppName]; exist {
				appname = str
			}

			// Storage
			podVolumeStr := ""
			if volumes := utils.GetPodStorage(&pod); volumes != nil {
				for i, volume := range volumes.Volumes {
					volumeQuantity := resource.NewQuantity(volume.Size, resource.BinarySI)
					volumeStr := fmt.Sprintf("<%d> %s: %s", i, volume.Kind, volumeQuantity.String())
					podVolumeStr = podVolumeStr + volumeStr
					if i+1 != len(volumes.Volumes) {
						podVolumeStr = fmt.Sprintf("%s\n", podVolumeStr)
					}
				}
			}

			data := []string{
				node.Name,
				fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
				fmt.Sprintf("%s(%d%%)", cpuReq.String(), int64(fractionCpuReq)),
				fmt.Sprintf("%s(%d%%)", memoryReq.String(), int64(fractionMemoryReq)),
				podVolumeStr,
				appname,
			}
			podTable.Append(data)
		}
	}
	podTable.SetAutoMergeCellsByColumnIndex([]int{0})
	podTable.SetRowLine(true)
	podTable.SetAlignment(tablewriter.ALIGN_LEFT)
	podTable.Render() // Send output

	fmt.Println()

	// Step 2: report node info
	fmt.Println("Node Info")
	nodeTable := tablewriter.NewWriter(os.Stdout)
	nodeTable.SetHeader([]string{
		"Node",
		"CPU Allocatable",
		"CPU Requests",
		"Memory Allocatable",
		"Memory Requests",
		"Pod Count",
		"New Node",
	})

	for _, node := range nodes.Items {
		reqs, limits := utils.GetPodsTotalRequestsAndLimitsByNodeName(allPods.Items, node.Name)
		nodeCpuReq, _, nodeMemoryReq, _, _, _ :=
			reqs[corev1.ResourceCPU], limits[corev1.ResourceCPU], reqs[corev1.ResourceMemory], limits[corev1.ResourceMemory], reqs[corev1.ResourceEphemeralStorage], limits[corev1.ResourceEphemeralStorage]
		allocatable := node.Status.Allocatable
		nodeFractionCpuReq := float64(nodeCpuReq.MilliValue()) / float64(allocatable.Cpu().MilliValue()) * 100
		nodeFractionMemoryReq := float64(nodeMemoryReq.Value()) / float64(allocatable.Memory().Value()) * 100
		newNode := ""
		if _, exist := node.Labels[simontype.LabelNewNode]; exist {
			newNode = "√"
		}

		data := []string{
			node.Name,
			allocatable.Cpu().String(),
			fmt.Sprintf("%s(%d%%)", nodeCpuReq.String(), int64(nodeFractionCpuReq)),
			allocatable.Memory().String(),
			fmt.Sprintf("%s(%d%%)", nodeMemoryReq.String(), int64(nodeFractionMemoryReq)),
			fmt.Sprintf("%d", utils.CountPodOnTheNode(allPods, node.Name)),
			newNode,
		}
		nodeTable.Append(data)
	}
	nodeTable.SetRowLine(true)
	nodeTable.SetAlignment(tablewriter.ALIGN_LEFT)
	nodeTable.Render() // Send output
	fmt.Println()

	// Step 3: report node storage info
	fmt.Println("Node Storage Info")
	nodeStorageTable := tablewriter.NewWriter(os.Stdout)
	nodeStorageTable.SetHeader([]string{
		"Node",
		"Storage Kind",
		"Storage Name",
		"Storage Allocatable",
		"Storage Requests",
	})
	for _, node := range nodes.Items {
		if nodeStorageStr, exist := node.Annotations[simontype.AnnoNodeLocalStorage]; exist {
			var nodeStorage utils.FakeNodeStorage
			if err := ffjson.Unmarshal([]byte(nodeStorageStr), &nodeStorage); err != nil {
				log.Fatalf("err when unmarshal json data, node is %s\n", node.Name)
			}
			var storageData []string
			for _, vg := range nodeStorage.VGs {
				capacity := resource.NewQuantity(vg.Capacity, resource.BinarySI)
				request := resource.NewQuantity(vg.Requested, resource.BinarySI)
				storageData = []string{
					node.Name,
					"VG",
					vg.Name,
					capacity.String(),
					fmt.Sprintf("%s(%d%%)", request.String(), int64(float64(vg.Requested)/float64(vg.Capacity)*100)),
				}
				nodeStorageTable.Append(storageData)
			}

			for _, device := range nodeStorage.Devices {
				capacity := resource.NewQuantity(device.Capacity, resource.BinarySI)
				used := "unused"
				if device.IsAllocated {
					used = "used"
				}
				storageData = []string{
					node.Name,
					"Device",
					device.Device,
					capacity.String(),
					used,
				}
				nodeStorageTable.Append(storageData)
			}
		}
	}
	nodeStorageTable.SetAutoMergeCellsByColumnIndex([]int{0})
	nodeStorageTable.SetRowLine(true)
	nodeStorageTable.SetAlignment(tablewriter.ALIGN_LEFT)
	nodeStorageTable.Render() // Send output
}

func (sim *Simulator) GetFakeClient() externalclientset.Interface {
	return sim.fakeclient
}

// CreateConfigMapAndSaveItToFile will create a file to save results for later handling
func (sim *Simulator) CreateConfigMapAndSaveItToFile(fileName string) error {
	var resultDeployment map[string][]string = make(map[string][]string)
	var resultStatefulment map[string][]string = make(map[string][]string)

	allPods, _ := sim.fakeclient.CoreV1().Pods(corev1.NamespaceAll).List(sim.ctx, metav1.ListOptions{
		// FieldSelector not work
		// issue: https://github.com/kubernetes/client-go/issues/326#issuecomment-412993326
		// FieldSelector: "spec.nodeName=%s" + node.Name,
	})
	for _, pod := range allPods.Items {
		var (
			kind              string
			workloadName      string
			workloadNamespace string
			exist             bool
		)

		if pod.Annotations == nil {
			continue
		}
		if kind, exist = pod.Annotations[simontype.AnnoWorkloadKind]; !exist {
			continue
		}
		if workloadName, exist = pod.Annotations[simontype.AnnoWorkloadName]; !exist {
			continue
		}
		if workloadNamespace, exist = pod.Annotations[simontype.AnnoWorkloadNamespace]; !exist {
			continue
		}
		switch kind {
		case simontype.WorkloadKindDeployment:
			resultDeployment[fmt.Sprintf("%s/%s", workloadNamespace, workloadName)] = append(resultDeployment[fmt.Sprintf("%s/%s", workloadNamespace, workloadName)], pod.Spec.NodeName)
		case simontype.WorkloadKindStatefulSet:
			resultStatefulment[fmt.Sprintf("%s/%s", workloadNamespace, workloadName)] = append(resultStatefulment[fmt.Sprintf("%s/%s", workloadNamespace, workloadName)], pod.Spec.NodeName)
		default:
			continue
		}
	}

	utils.AdjustWorkloads(resultDeployment)
	utils.AdjustWorkloads(resultStatefulment)

	byteDeployment, _ := json.Marshal(resultDeployment)
	byteStatefulment, _ := json.Marshal(resultStatefulment)
	var resultForCM map[string]string = make(map[string]string)
	resultForCM[simontype.WorkloadKindDeployment] = string(byteDeployment)
	resultForCM[simontype.WorkloadKindStatefulSet] = string(byteStatefulment)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      simontype.ConfigMapName,
			Namespace: simontype.ConfigMapNamespace,
		},
		Data: resultForCM,
	}

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return err
	}
	codec := serializer.NewCodecFactory(scheme).LegacyCodec(corev1.SchemeGroupVersion)
	output, _ := runtime.Encode(codec, configMap)
	if err := ioutil.WriteFile(fileName, output, 0644); err != nil {
		return err
	}

	return nil
}

// GetNodes
func (sim *Simulator) GetNodes() []corev1.Node {
	nodes, err := sim.fakeclient.CoreV1().Nodes().List(sim.ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Printf("GetNodes failed: %s\n", err.Error())
		os.Exit(1)
	}
	return nodes.Items
}

func (sim *Simulator) GetDaemonSets() []apps.DaemonSet {

	daemonsets, _ := sim.fakeclient.AppsV1().DaemonSets(metav1.NamespaceAll).List(sim.ctx, metav1.ListOptions{})
	return daemonsets.Items
}

func (sim *Simulator) Close() {
	sim.cancelFunc()
	close(sim.simulatorStop)
}

func (sim *Simulator) AddPods(pods []*corev1.Pod) error {
	for _, pod := range pods {
		_, err := sim.fakeclient.CoreV1().Pods(pod.Namespace).Create(context.Background(), pod, metav1.CreateOptions{})
		if err != nil {
			log.Errorf("create pod error: %s", err.Error())
		}
	}
	return nil
}

func (sim *Simulator) AddNodes(nodes []*corev1.Node) error {
	for _, node := range nodes {
		_, err := sim.fakeclient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (sim *Simulator) AddNewNode(node *corev1.Node, nodeCount int) error {
	if node == nil {
		return fmt.Errorf("node is nil")
	}

	fmt.Printf(utils.ColorYellow+"add %d node(s)\n"+utils.ColorReset, nodeCount)

	// make fake nodes
	for i := 0; i < nodeCount; i++ {
		hostname := fmt.Sprintf("%s-%02d", simontype.NewNodeNamePrefix, i)
		node := utils.MakeValidNodeByNode(node, hostname)
		metav1.SetMetaDataLabel(&node.ObjectMeta, simontype.LabelNewNode, "")
		_, err := sim.fakeclient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

// CreateFakeCluster synchronize cluster information to fake(simulated) cluster by kube-client or cluster configuration files
func (sim *Simulator) CreateFakeCluster(clusterConfigPath string) error {
	var resourceList simontype.ResourceTypes
	var err error
	if !reflect.ValueOf(sim.externalclient).IsZero() {
		nodeItems, err := sim.externalclient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("unable to list nodes: %v", err)
		}
		for _, item := range nodeItems.Items {
			newItem := item
			resourceList.Nodes = append(resourceList.Nodes, &newItem)
		}

		podItems, err := sim.externalclient.CoreV1().Pods(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("unable to list pods: %v", err)
		}
		for _, item := range podItems.Items {
			newItem := item
			resourceList.Pods = append(resourceList.Pods, &newItem)
		}

		pdbItems, err := sim.externalclient.PolicyV1beta1().PodDisruptionBudgets(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("unable to list PDBs: %v", err)
		}
		for _, item := range pdbItems.Items {
			newItem := item
			resourceList.PodDisruptionBudgets = append(resourceList.PodDisruptionBudgets, &newItem)
		}

		serviceItems, err := sim.externalclient.CoreV1().Services(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("unable to list services: %v", err)
		}
		for _, item := range serviceItems.Items {
			newItem := item
			resourceList.Services = append(resourceList.Services, &newItem)
		}

		storageClassesItems, err := sim.externalclient.StorageV1().StorageClasses().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("unable to list storage classes: %v", err)
		}
		for _, item := range storageClassesItems.Items {
			newItem := item
			resourceList.StorageClasss = append(resourceList.StorageClasss, &newItem)
		}

		pvcItems, err := sim.externalclient.CoreV1().PersistentVolumeClaims(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("unable to list pvcs: %v", err)
		}
		for _, item := range pvcItems.Items {
			newItem := item
			resourceList.PersistentVolumeClaims = append(resourceList.PersistentVolumeClaims, &newItem)
		}

		rcItems, err := sim.externalclient.CoreV1().ReplicationControllers(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("unable to list RCs: %v", err)
		}
		for _, item := range rcItems.Items {
			newItem := item
			resourceList.ReplicationControllers = append(resourceList.ReplicationControllers, &newItem)
		}

		deploymentItems, err := sim.externalclient.AppsV1().Deployments(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("unable to list deployment: %v", err)
		}
		for _, item := range deploymentItems.Items {
			newItem := item
			resourceList.Deployments = append(resourceList.Deployments, &newItem)
		}

		replicaSetItems, err := sim.externalclient.AppsV1().ReplicaSets(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("unable to list replicas sets: %v", err)
		}
		for _, item := range replicaSetItems.Items {
			newItem := item
			resourceList.ReplicaSets = append(resourceList.ReplicaSets, &newItem)
		}

		statefulSetItems, err := sim.externalclient.AppsV1().StatefulSets(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("unable to list stateful sets: %v", err)
		}
		for _, item := range statefulSetItems.Items {
			newItem := item
			resourceList.StatefulSets = append(resourceList.StatefulSets, &newItem)
		}

		daemonSetItems, err := sim.externalclient.AppsV1().DaemonSets(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("unable to list daemon sets: %v", err)
		}
		for _, item := range daemonSetItems.Items {
			newItem := item
			metav1.SetMetaDataLabel(&newItem.ObjectMeta, simontype.LabelDaemonSetFromCluster, "")
			resourceList.DaemonSets = append(resourceList.DaemonSets, &newItem)
		}
	} else {
		resourceList, err = sim.genResourceListFromClusterConfig(clusterConfigPath)
		if err != nil {
			return fmt.Errorf("Failed to generate resource list from cluster config: %v ", err)
		}
	}
	return sim.syncResourceList(resourceList)
}

// genResourceListFromClusterConfig
func (sim *Simulator) genResourceListFromClusterConfig(path string) (simontype.ResourceTypes, error) {
	clusterFilePaths, err := utils.ParseFilePath(path)
	if err != nil {
		return simontype.ResourceTypes{}, fmt.Errorf("Failed to parse the cluster config path: %v ", err)
	}
	resourceList, err := utils.GetObjectsFromFiles(clusterFilePaths)
	if err != nil {
		return simontype.ResourceTypes{}, err
	}

	resourceList.Pods = utils.GetValidPodExcludeDaemonSet(&resourceList)
	for _, item := range resourceList.DaemonSets {
		metav1.SetMetaDataLabel(&item.ObjectMeta, simontype.LabelDaemonSetFromCluster, "")
		resourceList.Pods = append(resourceList.Pods, utils.MakeValidPodsByDaemonset(item, resourceList.Nodes)...)
	}

	return resourceList, nil
}

func (sim *Simulator) syncResourceList(resourceList simontype.ResourceTypes) error {
	//sync node
	for _, item := range resourceList.Nodes {
		if _, err := sim.fakeclient.CoreV1().Nodes().Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("unable to copy node: %v", err)
		}
	}

	//sync pdb
	for _, item := range resourceList.PodDisruptionBudgets {
		if _, err := sim.fakeclient.PolicyV1beta1().PodDisruptionBudgets(item.Namespace).Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("unable to copy PDB: %v", err)
		}
	}

	//sync svc
	for _, item := range resourceList.Services {
		if _, err := sim.fakeclient.CoreV1().Services(item.Namespace).Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("unable to copy service: %v", err)
		}
	}

	//sync storage class
	for _, item := range resourceList.StorageClasss {
		if _, err := sim.fakeclient.StorageV1().StorageClasses().Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("unable to copy storage class: %v", err)
		}
	}

	//sync pvc
	for _, item := range resourceList.PersistentVolumeClaims {
		if _, err := sim.fakeclient.CoreV1().PersistentVolumeClaims(item.Namespace).Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("unable to copy pvc: %v", err)
		}
	}

	//sync rc
	for _, item := range resourceList.ReplicationControllers {
		if _, err := sim.fakeclient.CoreV1().ReplicationControllers(item.Namespace).Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("unable to copy RC: %v", err)
		}
	}

	//sync deployment
	for _, item := range resourceList.Deployments {
		if _, err := sim.fakeclient.AppsV1().Deployments(item.Namespace).Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("unable to copy deployment: %v", err)
		}
	}

	//sync rs
	for _, item := range resourceList.ReplicaSets {
		if _, err := sim.fakeclient.AppsV1().ReplicaSets(item.Namespace).Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("unable to copy replica set: %v", err)
		}
	}

	//sync statefulset
	for _, item := range resourceList.StatefulSets {
		if _, err := sim.fakeclient.AppsV1().StatefulSets(item.Namespace).Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("unable to copy stateful set: %v", err)
		}
	}

	//sync daemonset
	for _, item := range resourceList.DaemonSets {
		if _, err := sim.fakeclient.AppsV1().DaemonSets(item.Namespace).Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("unable to copy daemon set: %v", err)
		}
	}

	// sync pods
	if _, err := sim.SchedulePods(resourceList.Pods); err != nil {
		return err
	}

	return nil
}

// GenerateValidDaemonPodsForNewNode generates daemon pods after adding new node according to daemonset from cluster
func (sim *Simulator) GenerateValidDaemonPodsForNewNode() []*corev1.Pod {
	var pods []*corev1.Pod
	var fakeNodes []*corev1.Node

	nodeItems, _ := sim.fakeclient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{LabelSelector: simontype.LabelNewNode})
	for _, item := range nodeItems.Items {
		newItem := item
		fakeNodes = append(fakeNodes, &newItem)
	}

	// get all pods from daemonset
	daemonsets, _ := sim.fakeclient.AppsV1().DaemonSets(corev1.NamespaceAll).List(context.Background(), metav1.ListOptions{LabelSelector: simontype.LabelDaemonSetFromCluster})
	for _, item := range daemonsets.Items {
		newItem := item
		pods = append(pods, utils.MakeValidPodsByDaemonset(&newItem, fakeNodes)...)
	}

	return pods
}

func (sim *Simulator) update(pod *corev1.Pod, schedulerName string) error {
	var stop bool = false
	var stopReason string
	var stopMessage string
	for _, podCondition := range pod.Status.Conditions {
		// log.Infof("podCondition %v", podCondition)
		stop = podCondition.Type == corev1.PodScheduled && podCondition.Status == corev1.ConditionFalse && podCondition.Reason == corev1.PodReasonUnschedulable
		if stop {
			stopReason = podCondition.Reason
			stopMessage = podCondition.Message
			// fmt.Printf("stop is true: %s %s\n", stopReason, stopMessage)
			break
		}
	}
	// Only for pending pods provisioned by simon
	if stop {
		sim.status.stopReason = fmt.Sprintf("failed to schedule pod (%s/%s), %d pod(s) are waited to be scheduled: %s: %s", pod.Namespace, pod.Name, sim.status.numOfRemainingPods, stopReason, stopMessage)
		// }
	} else {
		sim.status.numOfRemainingPods--
		if sim.status.numOfRemainingPods == 0 {
			sim.status.stopReason = simontype.StopReasonSuccess
		} else {
			sim.status.stopReason = simontype.StopReasonDoNotStop
		}
	}
	sim.simulatorStop <- struct{}{}

	return nil
}

func (sim *Simulator) DoesClusterMeetRequirements() (bool, string) {
	nodes, _ := sim.fakeclient.CoreV1().Nodes().List(sim.ctx, metav1.ListOptions{})
	allPods, _ := sim.fakeclient.CoreV1().Pods(corev1.NamespaceAll).List(sim.ctx, metav1.ListOptions{})

	var err error
	var maxcpu int = 100
	var maxmem int = 100
	var maxvg int = 100
	if str := os.Getenv(simontype.EnvMaxCPU); str != "" {
		if maxcpu, err = strconv.Atoi(str); err != nil {
			log.Fatalf("convert env %s to int failed: %s", simontype.EnvMaxCPU, err.Error())
		}
		if maxcpu > 100 || maxcpu < 0 {
			maxcpu = 100
		}
	}

	if str := os.Getenv(simontype.EnvMaxMemory); str != "" {
		if maxmem, err = strconv.Atoi(str); err != nil {
			log.Fatalf("convert env %s to int failed: %s", simontype.EnvMaxMemory, err.Error())
		}
		if maxmem > 100 || maxmem < 0 {
			maxmem = 100
		}
	}

	if str := os.Getenv(simontype.EnvMaxVG); str != "" {
		if maxvg, err = strconv.Atoi(str); err != nil {
			log.Fatalf("convert env %s to int failed: %s", simontype.EnvMaxVG, err.Error())
		}
		if maxvg > 100 || maxvg < 0 {
			maxvg = 100
		}
	}

	for _, node := range nodes.Items {
		reqs, limits := utils.GetPodsTotalRequestsAndLimitsByNodeName(allPods.Items, node.Name)
		nodeCpuReq, _, nodeMemoryReq, _, _, _ :=
			reqs[corev1.ResourceCPU], limits[corev1.ResourceCPU], reqs[corev1.ResourceMemory], limits[corev1.ResourceMemory], reqs[corev1.ResourceEphemeralStorage], limits[corev1.ResourceEphemeralStorage]
		allocatable := node.Status.Allocatable
		nodeFractionCpuReq := int64(float64(nodeCpuReq.MilliValue()) / float64(allocatable.Cpu().MilliValue()) * 100)
		nodeFractionMemoryReq := int64(float64(nodeMemoryReq.Value()) / float64(allocatable.Memory().Value()) * 100)
		log.Debugf("node %s nodeFractionCpuReq %d nodeFractionMemoryReq %d", node.Name, nodeFractionCpuReq, nodeFractionMemoryReq)
		log.Debugf("maxcpu %d maxmem %d", maxcpu, maxmem)
		if nodeFractionCpuReq > int64(maxcpu) {
			return false, fmt.Sprintf("cpu usage of node %s is %d%%, but env %s is %d", node.Name, nodeFractionCpuReq, simontype.EnvMaxCPU, maxcpu)
		}
		if nodeFractionMemoryReq > int64(maxmem) {
			return false, fmt.Sprintf("memory usage of node %s is %d%%, but env %s is %d", node.Name, nodeFractionMemoryReq, simontype.EnvMaxMemory, maxmem)
		}

		if nodeStorageStr, exist := node.Annotations[simontype.AnnoNodeLocalStorage]; exist {
			var nodeStorage utils.FakeNodeStorage
			if err := ffjson.Unmarshal([]byte(nodeStorageStr), &nodeStorage); err != nil {
				log.Fatalf("err when unmarshal json data, node is %s\n", node.Name)
			}
			for _, vg := range nodeStorage.VGs {
				fraction := int(float64(vg.Requested) / float64(vg.Capacity) * 100)
				if fraction > maxvg {
					return false, fmt.Sprintf("vg %s usage of node %s is %d%%, but env %s is %d", vg.Name, node.Name, fraction, simontype.EnvMaxVG, maxvg)
				}
			}
		}
	}
	return true, ""
}
