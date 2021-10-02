package apply

import (
	"github.com/spf13/pflag"
)

// Options is the combined set of options for all operating modes.
type Options struct {
	Kubeconfig                 string
	DefaultSchedulerConfigFile string
	FilePath                   string
	UseBreed                   bool
}

// AddFlags will add the flag to the pflag.FlagSet
func (options *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&options.Kubeconfig, "kubeconfig", options.Kubeconfig, "Path to the kubeconfig file to use for the analysis.")
	fs.StringVar(&options.DefaultSchedulerConfigFile, "default-scheduler-config", options.DefaultSchedulerConfigFile, "Path to JSON or YAML file containing scheduler configuration.")
	fs.StringVarP(&options.FilePath, "filepath", "f", options.FilePath, "path that contains the configuration to apply")
	fs.BoolVar(&options.UseBreed, "use-greed", true, "use greedy algorithm when queue pods")
}
