package debug

import (
	"context"
	"fmt"
	"os"

	"github.com/alibaba/open-simulator/pkg/utils"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var options = Options{}

// DebugCmd is only for debug
var DebugCmd = &cobra.Command{
	Use:   "debug",
	Short: "debug alpha feature",
	Run: func(cmd *cobra.Command, args []string) {
		if err := run(&options); err != nil {
			fmt.Printf("debug error: %s", err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	options.AddFlags(DebugCmd.Flags())
	if err := DebugCmd.MarkFlagRequired("filepath"); err != nil {
		fmt.Printf("debug init error: %s", err.Error())
		os.Exit(1)
	}
}

func run(opt *Options) error {
	var err error
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
	allPods, _ := kubeClient.CoreV1().Pods(corev1.NamespaceAll).List(context.Background(), metav1.ListOptions{
		LabelSelector: "alibabacloud.com/qos=BE",
	})
	for _, pod := range allPods.Items {
		fmt.Printf("debug pod %s/%s\n", pod.Namespace, pod.Name)
	}

	return nil
}
