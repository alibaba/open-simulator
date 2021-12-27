package reschedule

import (
	"fmt"
	"os"

	reschedulepkg "github.com/alibaba/open-simulator/pkg/reschedule"
	reschedulerunner "github.com/alibaba/open-simulator/pkg/reschedule/runner"
	"github.com/spf13/cobra"
)

var options = Options{}

var RescheduleCmd = &cobra.Command{
	Use:   "reschedule",
	Short: "reschedule",
	Run: func(cmd *cobra.Command, args []string) {
		rescheduler, err := reschedulepkg.NewRescheduler(options.Kubeconfig, reschedulerunner.NewDefaultRunner())
		if err != nil {
			fmt.Printf("failed to init reschedule: %s", err.Error())
			os.Exit(1)
		}
		if err := rescheduler.Run(); err != nil {
			fmt.Printf("failed to run reschedule: %s", err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	options.AddFlags(RescheduleCmd.Flags())
	if err := RescheduleCmd.MarkFlagRequired("kubeconfig"); err != nil {
		fmt.Printf("reschedule init error: %s", err.Error())
		os.Exit(1)
	}
}
