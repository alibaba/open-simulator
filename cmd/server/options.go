package server

import (
	"github.com/spf13/pflag"
)

// Options is the combined set of options for all operating modes.
type Options struct {
	Kubeconfig string
	Master     string
}

// AddFlags will add the flag to the pflag.FlagSet
func (options *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&options.Kubeconfig, "kubeconfig", options.Kubeconfig, "Path to the kubeconfig file to use for the analysis.")
	fs.StringVar(&options.Master, "master", options.Master, "URL/IP for master.")
}
