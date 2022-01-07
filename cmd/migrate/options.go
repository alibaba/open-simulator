package migrate

import (
	"github.com/spf13/pflag"
)

// Options is the combined set of options for all operating modes.
type Options struct {
	KubeConfig                string
	RemoveList                []string
	LabelFilter               []string
	MaximumAverageUtilization int
}

// AddFlags will add the flag to the pflag.FlagSet
func (options *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&options.KubeConfig, "kube-config", options.KubeConfig, "path to the kube-config file to use for rescheduling")
	fs.StringSliceVarP(&options.RemoveList, "nodes-to-be-removed", "n", options.RemoveList, "you can input a few names of the nodes you want to remove to get a simulated result")
	fs.StringSliceVarP(&options.LabelFilter, "label-filter", "l", options.LabelFilter, "filter the pods you don't want to migrate")
	fs.IntVarP(&options.MaximumAverageUtilization, "maximum-average-utilization", "u", 100, "the upper limit of the resource utilization after rescheduling")

}
