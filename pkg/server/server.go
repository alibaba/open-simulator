package server

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

	"net/http"

	"github.com/alibaba/open-simulator/pkg/simulator"
	simontype "github.com/alibaba/open-simulator/pkg/type"
	"github.com/alibaba/open-simulator/pkg/utils"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	apimachineryjson "k8s.io/apimachinery/pkg/util/json"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	policylisters "k8s.io/client-go/listers/policy/v1beta1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type Server struct {
	nodeLister    corelisters.NodeLister
	podLister     corelisters.PodLister
	serviceLister corelisters.ServiceLister
	pvcLister     corelisters.PersistentVolumeClaimLister
	dsLister      appslisters.DaemonSetLister
	rsLister      appslisters.ReplicaSetLister
	stsLister     appslisters.StatefulSetLister
	pdbLister     policylisters.PodDisruptionBudgetLister
	scLister      storagelisters.StorageClassLister
	cmLister      corelisters.ConfigMapLister
}

type DeployAppRequest struct {
	// 待模拟调度的 Pod 信息
	Pods []*corev1.Pod `json:"pods"`
	// 待模拟调度的 Deployment 信息
	Deployments []*appsv1.Deployment `json:"deployments"`
	// 待模拟调度的 DaemonSet 信息
	DaemonSets []*appsv1.DaemonSet `json:"daemonsets"`
	// 待模拟调度的 StatefulSet 信息
	StatefulSets []*appsv1.StatefulSet `json:"statefulsets"`
	// 待模拟调度的 Job 信息
	Jobs []*batchv1.Job
	// 应用 ConfigMap 信息
	ConfigMaps []*corev1.ConfigMap
	// 添加的虚拟节点
	NewNodes []*corev1.Node `json:"newnodes"`
}

type ScaleAppRequest struct {
	// 待扩容的 Deployment 信息
	Deployments []*appsv1.Deployment `json:"deployments"`
	// 待扩容的 DaemonSet 信息
	DaemonSets []*appsv1.DaemonSet `json:"daemonsets"`
	// 待扩容的 StatefulSet 信息
	StatefulSets []*appsv1.StatefulSet `json:"statefulsets"`
	// 添加的虚拟节点
	NewNodes []*corev1.Node `json:"newnodes"`
}

type SimulateResponse struct {
	UnscheduledPods []UnscheduledPod `json:"unscheduledPods"`
	NodeStatus      []NodeStatus     `json:"nodeStatus"`
}

// 无法成功调度的 Pod 信息
type UnscheduledPod struct {
	Pod    string `json:"pod"`
	Reason string `json:"reason"`
}

// 已成功调度的 Pod 信息
type NodeStatus struct {
	// 节点信息
	Node string `json:"node"`
	// 该节点上所有 Pod 信息
	Pods []string `json:"pods"`
}

func NewServer(kubeconfig, master string) (*Server, error) {
	cfg, err := clientcmd.BuildConfigFromFlags(master, kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("Error building kubeconfig: %s", err.Error())
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("Error building kubernetes clientset: %s", err.Error())
	}
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	cxt := context.TODO()
	synced := []cache.InformerSynced{
		kubeInformerFactory.Core().V1().Nodes().Informer().HasSynced,
		kubeInformerFactory.Core().V1().Pods().Informer().HasSynced,
		kubeInformerFactory.Core().V1().Services().Informer().HasSynced,
		kubeInformerFactory.Core().V1().PersistentVolumeClaims().Informer().HasSynced,
		kubeInformerFactory.Core().V1().ConfigMaps().Informer().HasSynced,
		kubeInformerFactory.Apps().V1().DaemonSets().Informer().HasSynced,
		kubeInformerFactory.Apps().V1().StatefulSets().Informer().HasSynced,
		kubeInformerFactory.Apps().V1().ReplicaSets().Informer().HasSynced,
		kubeInformerFactory.Policy().V1beta1().PodDisruptionBudgets().Informer().HasSynced,
		kubeInformerFactory.Storage().V1().StorageClasses().Informer().HasSynced,
	}
	kubeInformerFactory.Start(cxt.Done())
	if !cache.WaitForCacheSync(cxt.Done(), synced...) {
		return nil, fmt.Errorf("timed out waiting for resource cache to sync")
	}

	return &Server{
		nodeLister:    kubeInformerFactory.Core().V1().Nodes().Lister(),
		podLister:     kubeInformerFactory.Core().V1().Pods().Lister(),
		serviceLister: kubeInformerFactory.Core().V1().Services().Lister(),
		pvcLister:     kubeInformerFactory.Core().V1().PersistentVolumeClaims().Lister(),
		dsLister:      kubeInformerFactory.Apps().V1().DaemonSets().Lister(),
		stsLister:     kubeInformerFactory.Apps().V1().StatefulSets().Lister(),
		rsLister:      kubeInformerFactory.Apps().V1().ReplicaSets().Lister(),
		pdbLister:     kubeInformerFactory.Policy().V1beta1().PodDisruptionBudgets().Lister(),
		scLister:      kubeInformerFactory.Storage().V1().StorageClasses().Lister(),
		cmLister:      kubeInformerFactory.Core().V1().ConfigMaps().Lister(),
	}, nil
}

func (server *Server) Start(opts ...simulator.Option) {
	r := server.setupRouter(opts...)

	// listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
	if err := r.Run(); err != nil {
		panic(err)
	}
}

func (server *Server) setupRouter(opts ...simulator.Option) *gin.Engine {
	defer utilruntime.HandleCrash()
	r := gin.Default()

	// check if server is healthy
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "ok",
		})
	})

	// deloy apps
	r.POST("api/deploy-apps", func(c *gin.Context) {
		// unmarshal
		req := &DeployAppRequest{}
		reqData, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, err.Error())
			return
		}
		if err := apimachineryjson.Unmarshal(reqData, req); err != nil {
			c.JSON(http.StatusBadRequest, fmt.Sprintf("fail to unmarshal content: %s\n", err.Error()))
			return
		}

		// get current cluster resources
		ClusterResource, err := server.getCurrentClusterResource()
		if err != nil {
			c.JSON(http.StatusInternalServerError, fmt.Sprintf("fail to get current cluster resources: %s", err.Error()))
			return
		}
		for _, newNode := range req.NewNodes {
			node, err := utils.NewFakeNode(newNode)
			if err != nil {
				c.JSON(http.StatusInternalServerError, fmt.Sprintf("fail to create a new fake node: %s", err.Error()))
				return
			}
			ClusterResource.Nodes = append(ClusterResource.Nodes, node)
		}

		// app resources
		AppResources := []simulator.AppResource{
			{
				Name: "test",
				Resource: simulator.ResourceTypes{
					Pods:         req.Pods,
					Deployments:  req.Deployments,
					StatefulSets: req.StatefulSets,
					DaemonSets:   req.DaemonSets,
					Jobs:         req.Jobs,
					ConfigMaps:   req.ConfigMaps,
				},
			},
		}
		pendingPods, err := server.getPendingPods()
		if err != nil {
			c.JSON(http.StatusInternalServerError, fmt.Sprintf("fail to get pending pods: %s", err.Error()))
			return
		}
		AppResources[0].Resource.Pods = append(AppResources[0].Resource.Pods, pendingPods...)

		// simulate
		result, err := simulator.Simulate(ClusterResource, AppResources, opts...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, err.Error())
			return
		}

		response := getSimulateResponse(result)

		c.JSON(http.StatusOK, response)
	})

	// scale apps
	r.POST("api/scale-apps", func(c *gin.Context) {
		// unmarshal
		req := &ScaleAppRequest{}
		reqData, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, err.Error())
			return
		}
		if err := apimachineryjson.Unmarshal(reqData, req); err != nil {
			c.JSON(http.StatusBadRequest, fmt.Sprintf("fail to unmarshal content: %s\n", err.Error()))
			return
		}

		// get current cluster resources
		ClusterResource, err := server.getCurrentClusterResource()
		if err != nil {
			c.JSON(http.StatusInternalServerError, fmt.Sprintf("fail to get current cluster resources: %s", err.Error()))
			return
		}
		for _, newNode := range req.NewNodes {
			node, err := utils.NewFakeNode(newNode)
			if err != nil {
				c.JSON(http.StatusInternalServerError, fmt.Sprintf("fail to create a new fake node: %s", err.Error()))
				return
			}
			ClusterResource.Nodes = append(ClusterResource.Nodes, node)
		}

		// remove app pods that will be scaled
		ClusterResource.Pods, err = server.removePodsOfApp(ClusterResource.Pods, req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, err.Error())
			return
		}
		for _, reqDaemonset := range req.DaemonSets {
			for j, ds := range ClusterResource.DaemonSets {
				if ds.Name == reqDaemonset.Name && ds.Namespace == reqDaemonset.Namespace {
					ClusterResource.DaemonSets[j] = reqDaemonset.DeepCopy()
					break
				}
			}
		}

		// app resources
		AppResources := []simulator.AppResource{
			{
				Name: "test",
				Resource: simulator.ResourceTypes{
					Deployments:  req.Deployments,
					StatefulSets: req.StatefulSets,
				},
			},
		}
		pendingPods, err := server.getPendingPods()
		if err != nil {
			c.JSON(http.StatusInternalServerError, fmt.Sprintf("fail to get pending pods: %s", err.Error()))
			return
		}
		pendingPods, err = server.removePodsOfApp(pendingPods, req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, err.Error())
			return
		}
		AppResources[0].Resource.Pods = append(AppResources[0].Resource.Pods, pendingPods...)

		// simulate
		result, err := simulator.Simulate(ClusterResource, AppResources, opts...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, err.Error())
			return
		}
		response := getSimulateResponse(result)

		c.JSON(http.StatusOK, response)
	})

	return r
}

func (server *Server) getPendingPods() ([]*corev1.Pod, error) {
	pendingPods := []*corev1.Pod{}
	pods, err := server.podLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list pods: %v", err)
	}
	for _, item := range pods {
		if !utils.OwnedByDaemonset(item.OwnerReferences) && item.DeletionTimestamp == nil && item.Status.Phase == corev1.PodPending {
			pendingPods = append(pendingPods, item.DeepCopy())
		}
	}
	return pendingPods, nil
}

func (server *Server) getCurrentClusterResource() (simulator.ResourceTypes, error) {
	var resource simulator.ResourceTypes
	var err error
	nodes, err := server.nodeLister.List(labels.Everything())
	if err != nil {
		return resource, fmt.Errorf("unable to list nodes: %v", err)
	}
	for _, item := range nodes {
		resource.Nodes = append(resource.Nodes, item.DeepCopy())
	}

	// We will regenerate pods of all workloads in the follow-up stage.
	pods, err := server.podLister.List(labels.Everything())
	if err != nil {
		return resource, fmt.Errorf("unable to list pods: %v", err)
	}
	for _, item := range pods {
		if !utils.OwnedByDaemonset(item.OwnerReferences) && item.DeletionTimestamp == nil && (item.Status.Phase == corev1.PodRunning) {
			resource.Pods = append(resource.Pods, item.DeepCopy())
		}
	}

	pdbs, err := server.pdbLister.List(labels.Everything())
	if err != nil {
		return resource, fmt.Errorf("unable to list PDBs: %v", err)
	}
	for _, item := range pdbs {
		resource.PodDisruptionBudgets = append(resource.PodDisruptionBudgets, item.DeepCopy())
	}

	services, err := server.serviceLister.List(labels.Everything())
	if err != nil {
		return resource, fmt.Errorf("unable to list services: %v", err)
	}
	for _, item := range services {
		resource.Services = append(resource.Services, item.DeepCopy())
	}

	storageClasses, err := server.scLister.List(labels.Everything())
	if err != nil {
		return resource, fmt.Errorf("unable to list storage classes: %v", err)
	}
	for _, item := range storageClasses {
		resource.StorageClasss = append(resource.StorageClasss, item.DeepCopy())
	}

	pvcs, err := server.pvcLister.List(labels.Everything())
	if err != nil {
		return resource, fmt.Errorf("unable to list pvcs: %v", err)
	}
	for _, item := range pvcs {
		resource.PersistentVolumeClaims = append(resource.PersistentVolumeClaims, item.DeepCopy())
	}

	daemonSets, err := server.dsLister.List(labels.Everything())
	if err != nil {
		return resource, fmt.Errorf("unable to list daemon sets: %v", err)
	}
	for _, item := range daemonSets {
		resource.DaemonSets = append(resource.DaemonSets, item.DeepCopy())
	}

	cms, err := server.cmLister.List(labels.Everything())
	if err != nil {
		return resource, fmt.Errorf("unable to list cm: %v", err)
	}
	for _, item := range cms {
		resource.ConfigMaps = append(resource.ConfigMaps, item.DeepCopy())
	}

	return resource, nil
}

func (server *Server) removePodsOfApp(pods []*corev1.Pod, apps *ScaleAppRequest) ([]*corev1.Pod, error) {
	var selectedObjs []runtime.Object
	newPods := []*corev1.Pod{}

	// deployment
	replicasets, err := server.rsLister.List(labels.Everything())
	if err != nil {
		return pods, err
	}
	for _, deploy := range apps.Deployments {
		for _, rs := range replicasets {
			if utils.OwnedByWorkload(rs.OwnerReferences, deploy) {
				selectedObjs = append(selectedObjs, rs.DeepCopy())
			}
		}
	}
	// statefulset
	for _, sts := range apps.StatefulSets {
		statefulset, err := server.stsLister.StatefulSets(sts.Namespace).Get(sts.Name)
		if err != nil {
			return pods, err
		}
		selectedObjs = append(selectedObjs, statefulset.DeepCopy())
	}

	// remove app
	for _, pod := range pods {
		owned := false
		for _, workload := range selectedObjs {
			if utils.OwnedByWorkload(pod.OwnerReferences, workload) {
				owned = true
				break
			}
		}
		if !owned {
			newPods = append(newPods, pod.DeepCopy())
		}
	}

	return newPods, nil
}

func getSimulateResponse(result *simulator.SimulateResult) *SimulateResponse {
	response := &SimulateResponse{}
	for _, pod := range result.UnscheduledPods {
		response.UnscheduledPods = append(response.UnscheduledPods, UnscheduledPod{
			Pod:    fmt.Sprintf("%s/%s", pod.Pod.Namespace, pod.Pod.Name),
			Reason: pod.Reason,
		})
	}
	for _, nodeStatus := range result.NodeStatus {
		pods := []string{}
		for _, pod := range nodeStatus.Pods {
			labels := pod.Labels
			if _, exist := labels[simontype.LabelAppName]; exist {
				pods = append(pods, fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
			}
		}
		if len(pods) != 0 {
			response.NodeStatus = append(response.NodeStatus, NodeStatus{
				Node: nodeStatus.Node.Name,
				Pods: pods,
			})
		}
	}
	return response
}
