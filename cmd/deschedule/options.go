package deschedule

import (
	"github.com/spf13/pflag"
)

// Options is the combined set of options for all operating modes.
type Options struct {
	Kubeconfig string
}

// AddFlags will add the flag to the pflag.FlagSet
func (options *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&options.Kubeconfig, "kubeconfig", options.Kubeconfig, "Path to the kubeconfig file to use for descheduling")
}
