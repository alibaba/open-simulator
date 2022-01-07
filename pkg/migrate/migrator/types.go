package migrator

import (
	"github.com/alibaba/open-simulator/pkg/simulator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodPlan struct {
	PodName      string
	PodNamespace string
	FromNode     string
	ToNode       string
	PodOwnerRefs []metav1.OwnerReference
}

type MigrationStatus struct {
	NodeName    string
	IsRemovable bool
	Reason      string
	PodPlans    []PodPlan
}

type MigrationResult struct {
	NodesMigrationStatus []MigrationStatus
}

type Migrator interface {
	Migrate(clusterResources simulator.ResourceTypes) (MigrationResult, error)
}
