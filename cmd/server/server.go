package server

import (
	"fmt"
	"os"

	serverpkg "github.com/alibaba/open-simulator/pkg/server"
	"github.com/spf13/cobra"
)

var options = Options{}

// ServerCmd is only for debug
var ServerCmd = &cobra.Command{
	Use:   "server",
	Short: "start a server to simulate deploying applications in k8s cluster",
	Run: func(cmd *cobra.Command, args []string) {
		if err := run(&options); err != nil {
			fmt.Printf("run server error: %s", err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	options.AddFlags(ServerCmd.Flags())
}

func run(opt *Options) error {
	server, err := serverpkg.NewServer(opt.Kubeconfig, opt.Master)
	if err != nil {
		return err
	}
	server.Start()
	return nil
}
