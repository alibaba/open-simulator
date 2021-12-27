package rescheduler

import (
	"context"
	"fmt"

	"github.com/alibaba/open-simulator/pkg/reschedule/runner"
	"github.com/alibaba/open-simulator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Rescheduler struct {
	runner     runner.Runner
	kubeclient *clientset.Clientset
}

func NewRescheduler(kubeconfig string, runner runner.Runner) (*Rescheduler, error) {
	// init kubeClient
	var cfg *restclient.Config
	master, err := utils.GetMasterFromKubeConfig(kubeconfig)
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
	return &Rescheduler{
		kubeclient: kubeClient,
		runner:     runner,
	}, nil
}

func (rescheduler *Rescheduler) Run() (err error) {
	// Step 1: get node list and pod list
	nodeList, err := rescheduler.kubeclient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list node: %v", err)
	}
	podList, err := rescheduler.kubeclient.CoreV1().Pods(corev1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list pod: %v", err)
	}

	// Step 2
	plans, err := rescheduler.runner.Run(nodeList.Items, podList.Items)
	if err != nil {
		return fmt.Errorf("failed to run runner: %v", err)
	}

	// Step 3
	rescheduler.report(plans)

	return nil
}

func (rescheduler *Rescheduler) report(plans []runner.ReschedulePlan) {
	if len(plans) == 0 {
		fmt.Println("No Removable Nodes")
		return
	}

	fmt.Println("Migration Plan")
	for _, plan := range plans {
		fmt.Printf("%s\n", plan.Node)
		for _, podPlan := range plan.PodPlans {
			fmt.Printf("\t%s/%s: %s -> %s\n", podPlan.PodNamespace, podPlan.PodName, podPlan.FromNode, podPlan.ToNode)
		}
	}
}
