package migrate

import (
	"context"
	"fmt"
	"os"

	"github.com/alibaba/open-simulator/pkg/migrate/migrator"
	"github.com/alibaba/open-simulator/pkg/simulator"
	"github.com/alibaba/open-simulator/pkg/utils"
	"github.com/olekukonko/tablewriter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Migrator struct {
	migrator migrator.Migrator
}

func NewMigrator(migrator migrator.Migrator) *Migrator {
	return &Migrator{
		migrator: migrator,
	}
}

func (migrator *Migrator) Run(kubeConfig string) error {
	var clusterResources simulator.ResourceTypes

	kubeclient, err := utils.CreateKubeClient(kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to create kubeclient: %v", err)
	}

	nodeItems, err := kubeclient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("unable to list nodes: %v", err)
	}
	for _, item := range nodeItems.Items {
		newItem := item
		clusterResources.Nodes = append(clusterResources.Nodes, &newItem)
	}

	podItems, err := kubeclient.CoreV1().Pods(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("unable to list pods: %v", err)
	}
	for _, item := range podItems.Items {
		newItem := item
		clusterResources.Pods = append(clusterResources.Pods, &newItem)
	}

	result, err := migrator.migrator.Migrate(clusterResources)
	if err != nil {
		return fmt.Errorf("failed to migrate pod: %v", err)
	}

	report(result)

	return nil
}

func report(rst migrator.MigrationResult) {
	if len(rst.NodesMigrationStatus) == 0 {
		fmt.Println(utils.ColorGreen + "No need to migrate pods" + utils.ColorReset)
	}

	plans := rst.NodesMigrationStatus
	for _, plan := range plans {
		fmt.Printf(utils.ColorYellow+"%s\n"+utils.ColorReset, plan.NodeName)
		if plan.IsRemovable {
			podMigrationTable := tablewriter.NewWriter(os.Stdout)
			podMigrationTable.SetHeader([]string{
				"Namespace",
				"Pod",
				"Destination Node",
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
		} else {
			fmt.Printf(utils.ColorRed+"Non-Migratable:\n%s"+utils.ColorReset, plan.Reason)
		}
	}
}
