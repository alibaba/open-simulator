package apply

import (
	"github.com/spf13/pflag"
)

// Options is the combined set of options for all operating modes.
type Options struct {
	KubeConfig                 string
	ClusterConfig              string
	AppConfig                  string
	DefaultSchedulerConfigFile string
	UseGreed                   bool
}

// AddFlags will add the flag to the pflag.FlagSet
func (options *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&options.KubeConfig, "kube-config", options.KubeConfig, "path to the cluster kube-config file used to connect cluster, one of both kube-config and cluster-config must exist.")
	fs.StringVar(&options.ClusterConfig,"cluster-config", options.ClusterConfig, "path to the directory of cluster configuration files to create a simulated cluster, one of both kube-config and cluster-config must exist.")
	fs.StringVarP(&options.AppConfig, "app-config", "f", options.AppConfig, "path to the application configuration as simulation resources to be scheduled")
	fs.StringVar(&options.DefaultSchedulerConfigFile, "default-scheduler-config", options.DefaultSchedulerConfigFile, "path to JSON or YAML file containing scheduler configuration.")
	fs.BoolVar(&options.UseGreed, "use-greed", true, "use greedy algorithm when queue pods")
}
