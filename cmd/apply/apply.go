package apply

import (
	"fmt"
	"os"
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
	"k8s.io/kubernetes/cmd/kube-scheduler/app/config"
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

	if err := ApplyCmd.MarkFlagRequired("app-config"); err != nil {
		log.Fatal("init ApplyCmd on app-config flag failed")
		return
	}
}

func run(opts *Options) error {
	// Step 0: check args
	if err := opts.checkArgs(); err != nil {
		return fmt.Errorf("Args Error: %v ", err)
	}

	// Step 1: convert recursively the application directory into a series of file paths
	appFilePaths, err := utils.ParseFilePath(opts.AppConfig)
	if err != nil {
		return fmt.Errorf("Failed to parse the application config path: %v ", err)
	}

	// Step 2: convert yml or yaml file of the application files to kubernetes resources
	resources, err := utils.GetObjectsFromFiles(appFilePaths)
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	//Field resources.Nodes will be a slice that only exists one node for the application configuration files
	//Field resources.Nodes will be a slice that exists one node at least for the cluster configuration files
	if len(resources.Nodes) != 1 {
		return fmt.Errorf("The number of nodes for the application files is not only one ")
	}

	// Step 3: generate kube-client
	kubeClient, err := generateKubeClient(opts.KubeConfig)
	if err != nil {
		return fmt.Errorf("Failed to get kubeclient: %v ", err)
	}

	// Step 4: get scheduler CompletedConfig and set the list of scheduler bind plugins to Simon.
	cc, err := getAndSetSchedulerConfig(opts.DefaultSchedulerConfigFile, opts.UseGreed)
	if err != nil {
		return err
	}

	// Step 5: get result
	for i := 0; i < 100; i++ {
		// 1: init simulator
		sim, err := simulator.New(kubeClient, cc, resources)
		if err != nil {
			return err
		}

		// load resources from real to fake
		if err := sim.SyncFakeCluster(opts.ClusterConfig); err != nil {
			return err
		}

		// add fake nodes
		if err := sim.AddFakeNode(i); err != nil {
			return err
		}

		// sync the application pods
		if err := sim.GenerateValidPodsFromResources(); err != nil {
			return err
		}

		// count pod without nodeName
		sim.CountPodsWithoutNodeName()

		// sort pods
		// TODO: These pods with nodeName have priority
		if opts.UseGreed {
			greed := algo.NewGreedQueue(sim.GetNodes(), sim.GetPodsToBeSimulated())
			sort.Sort(greed)
			// tol := algo.NewTolerationQueue(pods)
			// sort.Sort(tol)
			// aff := algo.NewAffinityQueue(pods)
			// sort.Sort(aff)
		}

		fmt.Printf(utils.ColorCyan+"There are %d pods to be scheduled\n"+utils.ColorReset, len(sim.GetPodsToBeSimulated()))
		err = sim.Run(sim.GetPodsToBeSimulated())
		if err != nil {
			return err
		}

		if sim.GetStatus() == simontype.StopReasonSuccess {
			fmt.Println(utils.ColorGreen + "Success!")
			sim.Report()
			if err := sim.CreateConfigMapAndSaveItToFile(simontype.ConfigMapFileName); err != nil {
				return err
			}
			break
		} else {
			fmt.Printf(utils.ColorRed+"Failed reason: %s\n"+utils.ColorReset, sim.GetStatus())
		}

		sim.Close()
	}
	fmt.Println(utils.ColorReset)
	return nil
}

// checkArgs checks whether parameters are valid
func (opts *Options) checkArgs() error {
	if len(opts.KubeConfig) == 0 && len(opts.ClusterConfig) == 0 || len(opts.KubeConfig) != 0 && len(opts.ClusterConfig) != 0 {
		return fmt.Errorf("only one of values of both kube-config and cluster-config must exist")
	}

	if opts.KubeConfig != "" {
		if _, err := os.Stat(opts.KubeConfig); err != nil {
			return fmt.Errorf("invalid path of kube-config: %v", err)
		}
	}

	if opts.ClusterConfig != "" {
		if _, err := os.Stat(opts.ClusterConfig); err != nil {
			return fmt.Errorf("invalid path of cluster-config: %v", err)
		}
	}

	if _, err := os.Stat(opts.AppConfig); err != nil {
		return fmt.Errorf("invalid path of app-config: %v", err)
	}

	return nil
}

// generateKubeClient generates kube-client by kube-config. And if kube-config file is not provided, the value of kube-client will be nil
func generateKubeClient(kubeConfigPath string) (*clientset.Clientset, error) {
	if len(kubeConfigPath) == 0 {
		return nil, nil
	}

	var err error
	var cfg *restclient.Config
	master, err := utils.GetMasterFromKubeConfig(kubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse kubeclient file: %v ", err)
	}

	cfg, err = clientcmd.BuildConfigFromFlags(master, kubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("Unable to build config: %v ", err)
	}
	//else {
	//	cfg, err = restclient.InClusterConfig()
	//	if err != nil {
	//		return nil, fmt.Errorf("Unable to build in cluster config: %v ", err)
	//	}
	//}
	kubeClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return kubeClient, nil
}

// getAndSetSchedulerConfig gets scheduler CompletedConfig and sets the list of scheduler bind plugins to Simon.
func getAndSetSchedulerConfig(defaultSchedulerConfigFile string, breed bool) (*config.CompletedConfig, error) {
	versionedCfg := kubeschedulerconfigv1beta1.KubeSchedulerConfiguration{}
	versionedCfg.DebuggingConfiguration = *configv1alpha1.NewRecommendedDebuggingConfiguration()
	kubeschedulerscheme.Scheme.Default(&versionedCfg)
	kcfg := kubeschedulerconfig.KubeSchedulerConfiguration{}
	if err := kubeschedulerscheme.Scheme.Convert(&versionedCfg, &kcfg, nil); err != nil {
		return nil, err
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

	if breed {
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
		ConfigFile:      defaultSchedulerConfigFile,
		Logs:            logs.NewOptions(),
	}
	cc, err := utils.InitKubeSchedulerConfiguration(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to init kube scheduler configuration: %v ", err)
	}
	return cc, nil
}
