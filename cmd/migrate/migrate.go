package migrate

import (
	"fmt"
	"os"

	"github.com/alibaba/open-simulator/pkg/migrate"
	"github.com/alibaba/open-simulator/pkg/migrate/migrator"
	"github.com/spf13/cobra"
)

var options = Options{}

var MigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "migrate",
	Run: func(cmd *cobra.Command, args []string) {
		migrator := migrate.NewMigrator(
			migrator.NewMigratorOnDownScaler(
				migrator.WithLabelFilter(options.LabelFilter),
				migrator.WithMaximumAverageUtilization(options.MaximumAverageUtilization),
				migrator.WithNodesToBeRemoved(options.RemoveList),
			),
		)

		if err := migrator.Run(options.KubeConfig); err != nil {
			fmt.Printf("failed to run migrator: %s", err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	options.AddFlags(MigrateCmd.Flags())
	if err := MigrateCmd.MarkFlagRequired("kube-config"); err != nil {
		fmt.Printf("migrator init error: %s", err.Error())
		os.Exit(1)
	}
}
