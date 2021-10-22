package apply

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"github.com/alibaba/open-simulator/pkg/algo"
	"github.com/alibaba/open-simulator/pkg/simulator"
	simontype "github.com/alibaba/open-simulator/pkg/type"
	"github.com/alibaba/open-simulator/pkg/utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	configv1alpha1 "k8s.io/component-base/config/v1alpha1"
	"k8s.io/component-base/logs"
	kubeschedulerconfigv1beta1 "k8s.io/kube-scheduler/config/v1beta1"
	schedoptions "k8s.io/kubernetes/cmd/kube-scheduler/app/options"
	kubeschedulerconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	kubeschedulerscheme "k8s.io/kubernetes/pkg/scheduler/apis/config/scheme"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
)

var options = Options{}

var ApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply a configuration to a resource by filename or stdin.",
	Run: func(cmd *cobra.Command, args []string) {
		if err := run(&options); err != nil {
			fmt.Printf("apply error: %s", err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	options.AddFlags(ApplyCmd.Flags())
	var err error
	err = ApplyCmd.MarkFlagRequired("kubeconfig")
	if err != nil {
		log.Fatal("init ApplyCmd on kubeconfig failed")
		return
	}
	err = ApplyCmd.MarkFlagRequired("filepath")
	if err != nil {
		log.Fatal("init ApplyCmd on filepath failed")
		return
	}
}

func run(opt *Options) error {
	// Step 0: variable declaration
	var err error
	var filePaths []string
	// Step 1: determining whether path points to file or directory
	if opt.FilePath != "" {
		fi, err := os.Stat(opt.FilePath)
		if err != nil {
			return err
		}
		switch mode := fi.Mode(); {
		case mode.IsDir():
			files, err := ioutil.ReadDir(opt.FilePath)
			if err != nil {
				return err
			}
			for _, f := range files {
				filePaths = append(filePaths, filepath.Join(opt.FilePath, f.Name()))
			}
		case mode.IsRegular():
			filePaths = append(filePaths, opt.FilePath)
		default:
			return fmt.Errorf("path is invalid")
		}
	}
	// Step 2: check
	resourceTypes := utils.GetObjectsFromFiles(filePaths)

	// Step 3: get kube client
	var cfg *restclient.Config
	if len(opt.Kubeconfig) != 0 {
		master, err := utils.GetMasterFromKubeConfig(opt.Kubeconfig)
		if err != nil {
			return fmt.Errorf("Failed to parse kubeconfig file: %v ", err)
		}

		cfg, err = clientcmd.BuildConfigFromFlags(master, opt.Kubeconfig)
		if err != nil {
			return fmt.Errorf("Unable to build config: %v", err)
		}
	} else {
		cfg, err = restclient.InClusterConfig()
		if err != nil {
			return fmt.Errorf("Unable to build in cluster config: %v", err)
		}
	}
	kubeClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		return err
	}

	// Step 4: get scheduler CompletedConfig.
	// Important: set the list of scheduler bind plugins to Simon
	versionedCfg := kubeschedulerconfigv1beta1.KubeSchedulerConfiguration{}
	versionedCfg.DebuggingConfiguration = *configv1alpha1.NewRecommendedDebuggingConfiguration()
	kubeschedulerscheme.Scheme.Default(&versionedCfg)
	kcfg := kubeschedulerconfig.KubeSchedulerConfiguration{}
	if err := kubeschedulerscheme.Scheme.Convert(&versionedCfg, &kcfg, nil); err != nil {
		return err
	}
	if len(kcfg.Profiles) == 0 {
		kcfg.Profiles = []kubeschedulerconfig.KubeSchedulerProfile{
			{},
		}
	}
	kcfg.Profiles[0].SchedulerName = corev1.DefaultSchedulerName
	if kcfg.Profiles[0].Plugins == nil {
		kcfg.Profiles[0].Plugins = &kubeschedulerconfig.Plugins{}
	}

	if opt.UseBreed {
		kcfg.Profiles[0].Plugins.Score = &kubeschedulerconfig.PluginSet{
			Enabled: []kubeschedulerconfig.Plugin{{Name: simontype.SimonPluginName}},
		}
	}
	kcfg.Profiles[0].Plugins.Bind = &kubeschedulerconfig.PluginSet{
		Enabled:  []kubeschedulerconfig.Plugin{{Name: simontype.SimonPluginName}},
		Disabled: []kubeschedulerconfig.Plugin{{Name: defaultbinder.Name}},
	}
	// set percentageOfNodesToScore value to 100
	kcfg.PercentageOfNodesToScore = 100
	opts := &schedoptions.Options{
		ComponentConfig: kcfg,
		ConfigFile:      opt.DefaultSchedulerConfigFile,
		Logs:            logs.NewOptions(),
	}
	cc, err := utils.InitKubeSchedulerConfiguration(opts)
	if err != nil {
		return fmt.Errorf("failed to init kube scheduler configuration: %v ", err)
	}

	// Step 5: get result
	for i := 0; i < 100; i++ {
		// 1: init simulator
		sim, err := simulator.New(kubeClient, cc, resourceTypes)
		if err != nil {
			return err
		}

		// load resources from real to fake
		if err := sim.SyncFakeCluster(); err != nil {
			return err
		}

		// add fake nodes
		if err := sim.AddFakeNode(i); err != nil {
			return err
		}

		// sync the pods of daemonset
		if err := sim.GenerateValidPodsFromResources(); err != nil {
			return err
		}

		// 2: run simulator instance
		if opt.UseBreed {
			greed := algo.NewGreedQueue(sim.GetNodes(), sim.GetPodsToBeSimulated())
			sort.Sort(greed)
			// tol := algo.NewTolerationQueue(pods)
			// sort.Sort(tol)
			// aff := algo.NewAffinityQueue(pods)
			// sort.Sort(aff)
		}

		fmt.Printf(string(utils.ColorCyan)+"There are %d pods to be scheduled\n"+string(utils.ColorReset), len(sim.GetPodsToBeSimulated()))
		err = sim.Run(sim.GetPodsToBeSimulated())
		if err != nil {
			return err
		}

		if sim.GetStatus() == simontype.StopReasonSuccess {
			fmt.Println(string(utils.ColorGreen) + "Success!")
			sim.Report()
			if err := sim.CreateConfigMapAndSaveItToFile(simontype.ConfigMapFileName); err != nil {
				return err
			}
			break
		} else {
			fmt.Printf(string(utils.ColorRed)+"Failed reason: %s\n"+string(utils.ColorReset), sim.GetStatus())
		}

		sim.Close()
	}
	fmt.Println(string(utils.ColorReset))
	return nil
}
