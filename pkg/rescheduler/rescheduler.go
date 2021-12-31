package rescheduler

import (
	"context"
	"fmt"
	"os"

	"github.com/alibaba/open-simulator/pkg/rescheduler/runner"
	"github.com/alibaba/open-simulator/pkg/utils"
	"github.com/olekukonko/tablewriter"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Rescheduler struct {
	runner runner.Runner
}

func NewRescheduler(runner runner.Runner) (*Rescheduler, error) {
	return &Rescheduler{
		runner: runner,
	}, nil
}

func (rescheduler *Rescheduler) PreRun(kubeConfig string) (err error) {
	kubeclient, err := utils.CreateKubeClient(kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to create kubeclient: %v", err)
	}

	// Step 1: get node list and pod list
	nodeList, err := kubeclient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list node: %v", err)
	}
	podList, err := kubeclient.CoreV1().Pods(corev1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
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

func (rescheduler *Rescheduler) report(plans []runner.MigrationPlan) {
	if len(plans) == 0 {
		fmt.Println("No Removable Nodes")
		return
	}

	for _, plan := range plans {
		fmt.Printf(utils.ColorYellow+"%s\n"+utils.ColorReset, plan.NodeName)
		podMigrationTable := tablewriter.NewWriter(os.Stdout)
		podMigrationTable.SetHeader([]string{
			"Namespace",
			"Pod",
			"To",
		})

		for _, podPlan := range plan.PodPlans {
			data := []string{
				podPlan.PodNamespace,
				podPlan.PodName,
				podPlan.ToNode,
			}

			podMigrationTable.Append(data)
		}

		podMigrationTable.SetRowLine(true)
		podMigrationTable.Render()
		fmt.Println()
	}
}
