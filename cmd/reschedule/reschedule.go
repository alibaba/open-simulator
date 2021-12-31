package reschedule

import (
	"fmt"
	"os"

	"github.com/alibaba/open-simulator/pkg/rescheduler"
	"github.com/alibaba/open-simulator/pkg/rescheduler/runner"
	"github.com/spf13/cobra"
)

var options = Options{}
var removerOptions = runner.RemoverOptions{
	LabelFilter: options.LabelFilter,
}

var RescheduleCmd = &cobra.Command{
	Use:   "reschedule",
	Short: "reschedule",
	Run: func(cmd *cobra.Command, args []string) {
		rescheduler, err := rescheduler.NewRescheduler(runner.NewRemover(removerOptions))
		if err != nil {
			fmt.Printf("failed to init reschedule: %s", err.Error())
			os.Exit(1)
		}
		if err := rescheduler.PreRun(options.KubeConfig); err != nil {
			fmt.Printf("failed to run reschedule: %s", err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	options.AddFlags(RescheduleCmd.Flags())
	if err := RescheduleCmd.MarkFlagRequired("kube-config"); err != nil {
		fmt.Printf("reschedule init error: %s", err.Error())
		os.Exit(1)
	}
}
