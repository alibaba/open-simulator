package descheduler

import (
	"context"
	"fmt"

	"github.com/alibaba/open-simulator/pkg/deschedule/runner"
	"github.com/alibaba/open-simulator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Descheduler struct {
	runner     runner.Runner
	kubeclient *clientset.Clientset
}

func NewDescheduler(kubeconfig string, runner runner.Runner) (*Descheduler, error) {
	// init kubeClient
	var cfg *restclient.Config
	master, err := utils.GetMasterFromKubeConfig(kubeconfig)
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
	return &Descheduler{
		kubeclient: kubeClient,
		runner:     runner,
	}, nil
}

func (descheduler *Descheduler) Run() (err error) {
	// Step 1: get node list and pod list
	nodeList, err := descheduler.kubeclient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	podList, err := descheduler.kubeclient.CoreV1().Pods(corev1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	// Step 2
	plans, err := descheduler.runner.Run(nodeList.Items, podList.Items)
	if err != nil {
		return err
	}

	// Step 3
	descheduler.report(plans)

	return nil
}

func (descheduler *Descheduler) report(plans []runner.DeschedulePlan) {
	for _, plan := range plans {
		fmt.Printf("migrate pod %s/%s from %s to %s\n", plan.PodNamespace, plan.PodName, plan.FromNode, plan.ToNode)
	}
}
