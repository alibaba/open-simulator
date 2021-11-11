package apply

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sigs.k8s.io/yaml"
	"sort"

	"github.com/alibaba/open-simulator/pkg/algo"
	"github.com/alibaba/open-simulator/pkg/chart"
	"github.com/alibaba/open-simulator/pkg/simulator"
	simontype "github.com/alibaba/open-simulator/pkg/type"
	"github.com/alibaba/open-simulator/pkg/utils"
	"github.com/alibaba/open-simulator/pkg/v1alpha1"
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

type Options struct {
	SimonConfig                string
	DefaultSchedulerConfigFile string
	UseGreed                   bool
	Interactive                bool
}

type DefaultApplier struct {
	cluster         v1alpha1.Cluster
	appList         []v1alpha1.AppInfo
	newNode         string
	schedulerConfig string
	useGreed        bool
	interactive     bool
}

func (applier *DefaultApplier) Run() (err error) {
	var resourceList []simontype.ResourceInfo

	// Step 1: convert a series of the application paths into the kubernetes objects
	for _, app := range applier.appList {
		newPath := app.Path

		if app.Chart {
			outputDir, err := chart.ProcessChart(app.Name, app.Path)
			if err != nil {
				return err
			}
			newPath = outputDir
		}

		// convert recursively the application directory into a series of file paths
		appFilePaths, err := utils.ParseFilePath(newPath)
		if err != nil {
			return fmt.Errorf("Failed to parse the application config path: %v ", err)
		}

		// convert yml or yaml file of the application files to kubernetes appResources
		appResource, err := utils.GetObjectsFromFiles(appFilePaths)
		if err != nil {
			return fmt.Errorf("%v", err)
		}

		newResource := simontype.ResourceInfo{
			Name:     app.Name,
			Resource: appResource,
		}
		resourceList = append(resourceList, newResource)
	}

	// Step 2: convert the path of the new node to be added into the kubernetes object
	objects := utils.DecodeYamlFile(applier.newNode)
	newNode, exist := objects[0].(*corev1.Node)
	if !exist {
		return fmt.Errorf("The NewNode file(%s) is not a Node yaml ", applier.newNode)
	}

	// Step 3: generate kube-client
	kubeClient, err := applier.generateKubeClient()
	if err != nil {
		return fmt.Errorf("Failed to get kubeclient: %v ", err)
	}

	// Step 4: get scheduler CompletedConfig and set the list of scheduler bind plugins to Simon.
	cc, err := applier.getAndSetSchedulerConfig()
	if err != nil {
		return err
	}

	// Step 5: get result
	for i := 0; i < 100; i++ {
		// init simulator
		sim, err := simulator.New(kubeClient, cc)
		if err != nil {
			return err
		}

		// start a scheduler as a goroutine
		sim.RunScheduler()

		// synchronize resources from real or simulated cluster to fake cluster
		if err := sim.CreateFakeCluster(applier.cluster.CustomCluster); err != nil {
			return fmt.Errorf("create fake cluster failed: %s", err.Error())
		}

		// add nodes to get a successful scheduling
		if err := sim.AddNewNode(newNode, i); err != nil {
			return err
		}

		// success: to determine whether the current resource is successfully scheduled
		// added: the daemon pods derived from the cluster daemonset only need to be added once
		success, added := false, false
		for _, resourceInfo := range resourceList {
			success = false
			// synchronize pods generated by deployment、daemonset and like this, then format all unscheduled pods
			appPods := simulator.GenerateValidPodsFromResources(sim.GetFakeClient(), resourceInfo.Resource)
			if !added {
				appPods = append(appPods, sim.GenerateValidDaemonPodsForNewNode()...)
				added = true
			}

			// sort pods
			if applier.useGreed {
				greed := algo.NewGreedQueue(sim.GetNodes(), appPods)
				sort.Sort(greed)
				// tol := algo.NewTolerationQueue(pods)
				// sort.Sort(tol)
				// aff := algo.NewAffinityQueue(pods)
				// sort.Sort(aff)
			}

			fmt.Printf(utils.ColorCyan+"%s: %d pods to be simulated, %d pods of which to be scheduled\n"+utils.ColorReset, resourceInfo.Name, len(appPods), utils.GetTotalNumberOfPodsWithoutNodeName(appPods))
			err = sim.SchedulePods(appPods)
			if err != nil {
				fmt.Printf(utils.ColorRed+"%s: %s\n"+utils.ColorReset, resourceInfo.Name, err.Error())
				break
			} else {
				success = true
				fmt.Printf(utils.ColorGreen+"%s: Success!", resourceInfo.Name)
				sim.Report()
				fmt.Println(utils.ColorReset)
				if err := sim.CreateConfigMapAndSaveItToFile(simontype.ConfigMapFileName); err != nil {
					return err
				}
				if applier.interactive {
					prompt := fmt.Sprintf("%s scheduled succeessfully, continue(y/n)?", resourceInfo.Name)
					if utils.Confirm(prompt) {
						continue
					} else {
						break
					}
				}
			}
		}
		sim.Close()

		if success {
			fmt.Printf(utils.ColorCyan + "Congratulations! A Successful Scheduling!" + utils.ColorReset)
			break
		}
	}
	return nil
}

// NewApplier returns a default applier that has passed the validity test
func NewApplier(opts Options) DefaultApplier {
	simonCR := &v1alpha1.Simon{}
	configFile, err := ioutil.ReadFile(opts.SimonConfig)
	if err != nil {
		log.Fatalf("failed to read config file(%s): %v", opts.SimonConfig, err)
	}
	configJSON, err := yaml.YAMLToJSON(configFile)
	if err != nil {
		log.Fatalf("failed to unmarshal config file(%s) to json: %v", opts.SimonConfig, err)
	}

	if err := json.Unmarshal(configJSON, simonCR); err != nil {
		log.Fatalf("failed to unmarshal config json to object: %v", err)
	}

	applier := DefaultApplier{
		cluster:         simonCR.Spec.Cluster,
		appList:         simonCR.Spec.AppList,
		newNode:         simonCR.Spec.NewNode,
		schedulerConfig: opts.DefaultSchedulerConfigFile,
		useGreed:        opts.UseGreed,
		interactive:     opts.Interactive,
	}

	if err := applier.Validate(); err != nil {
		fmt.Printf("%v", err)
		os.Exit(1)
	}

	return applier
}

// generateKubeClient generates kube-client by kube-config. And if kube-config file is not provided, the value of kube-client will be nil
func (applier *DefaultApplier) generateKubeClient() (*clientset.Clientset, error) {
	if len(applier.cluster.KubeConfig) == 0 {
		return nil, nil
	}

	var err error
	var cfg *restclient.Config
	master, err := utils.GetMasterFromKubeConfig(applier.cluster.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse kubeclient file: %v ", err)
	}

	cfg, err = clientcmd.BuildConfigFromFlags(master, applier.cluster.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("Unable to build config: %v ", err)
	}

	kubeClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return kubeClient, nil
}

// getAndSetSchedulerConfig gets scheduler CompletedConfig and sets the list of scheduler bind plugins to Simon.
func (applier *DefaultApplier) getAndSetSchedulerConfig() (*config.CompletedConfig, error) {
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

	if applier.useGreed {
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
		ConfigFile:      applier.schedulerConfig,
		Logs:            logs.NewOptions(),
	}
	cc, err := utils.InitKubeSchedulerConfiguration(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to init kube scheduler configuration: %v ", err)
	}
	return cc, nil
}

func (applier *DefaultApplier) Validate() error {
	if len(applier.cluster.KubeConfig) == 0 && len(applier.cluster.CustomCluster) == 0 ||
		len(applier.cluster.KubeConfig) != 0 && len(applier.cluster.CustomCluster) != 0 {
		return fmt.Errorf("only one of values of both kubeConfig and customConfig must exist")
	}

	if len(applier.cluster.KubeConfig) != 0 {
		if _, err := os.Stat(applier.cluster.KubeConfig); err != nil {
			return fmt.Errorf("invalid path of kubeConfig: %v", err)
		}
	}

	if len(applier.cluster.CustomCluster) != 0 {
		if _, err := os.Stat(applier.cluster.CustomCluster); err != nil {
			return fmt.Errorf("invalid path of customConfig: %v", err)
		}
	}

	if len(applier.schedulerConfig) != 0 {
		if _, err := os.Stat(applier.schedulerConfig); err != nil {
			return fmt.Errorf("invalid path of scheduler config: %v", err)
		}
	}

	if len(applier.newNode) != 0 {
		if _, err := os.Stat(applier.newNode); err != nil {
			return fmt.Errorf("invalid path of newNode: %v", err)
		}
	}

	for _, app := range applier.appList {
		if _, err := os.Stat(app.Path); err != nil {
			return fmt.Errorf("invalid path of %s app: %v", app.Name, err)
		}
	}

	return nil
}