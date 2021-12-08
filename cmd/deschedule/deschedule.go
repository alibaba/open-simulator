package deschedule

import (
	"fmt"
	"os"

	deschedulepkg "github.com/alibaba/open-simulator/pkg/deschedule"
	deschedulerunner "github.com/alibaba/open-simulator/pkg/deschedule/runner"
	"github.com/spf13/cobra"
)

var options = Options{}

var DescheduleCmd = &cobra.Command{
	Use:   "deschedule",
	Short: "deschedule",
	Run: func(cmd *cobra.Command, args []string) {
		descheduler, err := deschedulepkg.NewDescheduler(options.Kubeconfig, deschedulerunner.NewDefaultRunner())
		if err != nil {
			fmt.Printf("failed to init deschedule: %s", err.Error())
			os.Exit(1)
		}
		if err := descheduler.Run(); err != nil {
			fmt.Printf("failed to run deschedule: %s", err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	options.AddFlags(DescheduleCmd.Flags())
	if err := DescheduleCmd.MarkFlagRequired("kubeconfig"); err != nil {
		fmt.Printf("deschedule init error: %s", err.Error())
		os.Exit(1)
	}
}
