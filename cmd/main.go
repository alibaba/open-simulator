package main

import (
	"fmt"
	"os"

	goflag "flag"

	cliflag "k8s.io/component-base/cli/flag"

	"github.com/alibaba/open-simulator/cmd/apply"
	"github.com/alibaba/open-simulator/cmd/version"
	"github.com/spf13/cobra"
)

var SimonCmd = &cobra.Command{
	Use:   "simon",
	Short: "Simon is a simulator, which will simulate a cluster and simulate workload scheduling.",
}

func main() {
	if err := SimonCmd.Execute(); err != nil {
		fmt.Printf("start with error: %s", err.Error())
		os.Exit(1)
	}
}

func init() {
	addCommands()
	SimonCmd.SetGlobalNormalizationFunc(cliflag.WordSepNormalizeFunc)
	SimonCmd.Flags().AddGoFlagSet(goflag.CommandLine)
}

func addCommands() {
	SimonCmd.AddCommand(
		version.VersionCmd,
		apply.ApplyCmd,
	)
}
