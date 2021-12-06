package simulator

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	localcache "github.com/alibaba/open-local/pkg/scheduler/algorithm/cache"
	"github.com/alibaba/open-simulator/pkg/test"
	simontype "github.com/alibaba/open-simulator/pkg/type"
	"github.com/alibaba/open-simulator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type args struct {
	cluster ResourceTypes
	apps    []AppResource
	opts    []Option
}

type workload struct {
	name      string
	namespace string
	kind      string
}

func TestSimulate(t *testing.T) {
	tests := []struct {
		name          string
		args          args
		wantErr       bool
		failedPodsNum int64
	}{
		// TODO: Add test cases.
		{
			name: "simple",
			args: args{
				cluster: ResourceTypes{
					Nodes: []*corev1.Node{
						// master-1
						test.MakeFakeNode("master-1", "8", "16Gi",
							test.WithNodeLabels(map[string]string{
								"beta.kubernetes.io/arch":        "amd64",
								"beta.kubernetes.io/os":          "linux",
								"kubernetes.io/arch":             "amd64",
								"kubernetes.io/hostname":         "master-1",
								"kubernetes.io/os":               "linux",
								"node-role.kubernetes.io/master": "",
							}),
							test.WithNodeTaints([]corev1.Taint{
								{
									Key:    "node-role.kubernetes.io/master",
									Effect: corev1.TaintEffectNoSchedule,
								},
							}),
							test.WithNodeLocalStorage(utils.NodeStorage{
								VGs: []localcache.SharedResource{
									{
										Name:     "yoda-pool0",
										Capacity: 107374182400,
									},
									{
										Name:     "yoda-pool1",
										Capacity: 107374182400,
									},
								},
								Devices: []localcache.ExclusiveResource{
									{
										Name:        "/dev/vdd",
										Device:      "/dev/vdd",
										Capacity:    107374182400,
										IsAllocated: false,
										MediaType:   "hdd",
									},
								},
							}),
						),
						// master-2
						test.MakeFakeNode("master-2", "8", "16Gi",
							test.WithNodeLabels(map[string]string{
								"beta.kubernetes.io/arch":        "amd64",
								"beta.kubernetes.io/os":          "linux",
								"kubernetes.io/arch":             "amd64",
								"kubernetes.io/hostname":         "master-2",
								"kubernetes.io/os":               "linux",
								"node-role.kubernetes.io/master": "",
							}),
						),
						// master-3
						test.MakeFakeNode("master-3", "8", "16Gi",
							test.WithNodeLabels(map[string]string{
								"beta.kubernetes.io/arch":        "amd64",
								"beta.kubernetes.io/os":          "linux",
								"kubernetes.io/arch":             "amd64",
								"kubernetes.io/hostname":         "master-3",
								"kubernetes.io/os":               "linux",
								"node-role.kubernetes.io/master": "",
							}),
						),
						// worker-1
						test.MakeFakeNode("worker-1", "8", "16Gi",
							test.WithNodeLabels(map[string]string{
								"beta.kubernetes.io/arch":        "amd64",
								"beta.kubernetes.io/os":          "linux",
								"kubernetes.io/arch":             "amd64",
								"kubernetes.io/hostname":         "worker-1",
								"kubernetes.io/os":               "linux",
								"node-role.kubernetes.io/worker": "",
							}),
							test.WithNodeLocalStorage(utils.NodeStorage{
								VGs: []localcache.SharedResource{
									{
										Name:     "yoda-pool0",
										Capacity: 107374182400,
									},
									{
										Name:     "yoda-pool1",
										Capacity: 107374182400,
									},
								},
								Devices: []localcache.ExclusiveResource{
									{
										Name:        "/dev/vdd",
										Device:      "/dev/vdd",
										Capacity:    107374182400,
										IsAllocated: false,
										MediaType:   "hdd",
									},
								},
							}),
						),
					},
					Pods: []*corev1.Pod{
						test.MakeFakePod("etcd-master-1", "kube-system", "", "",
							test.WithPodNodeName("master-1"),
						),
						test.MakeFakePod("kube-apiserver-master-1", "kube-system", "250m", "",
							test.WithPodNodeName("master-1"),
						),
						test.MakeFakePod("kube-controller-manager-master-1", "kube-system", "200m", "",
							test.WithPodNodeName("master-1"),
						),
						test.MakeFakePod("kube-scheduler-master-1", "kube-system", "100m", "",
							test.WithPodNodeName("master-1"),
						),
					},
					Deployments: []*appsv1.Deployment{
						// metrics-server
						test.MakeFakeDeployment("metrics-server", "kube-system", 1, "1", "500Mi",
							test.WithDeploymentAffinity(&corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "node-role.kubernetes.io/master",
														Operator: corev1.NodeSelectorOpExists,
													},
												},
											},
										},
									},
								},
								PodAntiAffinity: &corev1.PodAntiAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
										{
											LabelSelector: &metav1.LabelSelector{
												MatchLabels: map[string]string{
													"k8s-app": "metrics-server",
												},
											},
											TopologyKey: "failure-domain.beta.kubernetes.io/zone",
										},
									},
								},
							})),
					},
					DaemonSets: []*appsv1.DaemonSet{
						// kube-proxy-master
						test.MakeFakeDaemonSet("kube-proxy-master", "kube-system", "", "",
							test.WithDaemonSetTolerations([]corev1.Toleration{
								{
									Operator: corev1.TolerationOpExists,
								},
							}),
							test.WithDaemonSetNodeSelector(map[string]string{
								"node-role.kubernetes.io/master": "",
							}),
						),
						// kube-proxy-worker
						test.MakeFakeDaemonSet("kube-proxy-worker", "kube-system", "", "",
							test.WithDaemonSetTolerations([]corev1.Toleration{
								{
									Operator: corev1.TolerationOpExists,
								},
							}),
							test.WithDaemonSetNodeSelector(map[string]string{
								"node-role.kubernetes.io/worker": "",
							}),
						),
						// coredns
						test.MakeFakeDaemonSet("coredns", "kube-system", "100m", "70Mi",
							test.WithDaemonSetAffinity(&corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "node-role.kubernetes.io/master",
														Operator: corev1.NodeSelectorOpExists,
													},
												},
											},
										},
									},
								},
							}),
							test.WithDaemonSetTolerations([]corev1.Toleration{
								{
									Effect: corev1.TaintEffectNoSchedule,
									Key:    "node-role.kubernetes.io/master",
								},
							}),
							test.WithDaemonSetNodeSelector(map[string]string{
								"beta.kubernetes.io/os": "linux",
							}),
						),
					},
				},
				// simple
				apps: []AppResource{
					{
						Name: "simple",
						Resource: ResourceTypes{
							Deployments: []*appsv1.Deployment{
								test.MakeFakeDeployment("busybox-deploy", "simple", 4, "1500m", "1Gi",
									test.WithDeploymentTolerations([]corev1.Toleration{
										{
											Effect:   corev1.TaintEffectNoSchedule,
											Key:      "node-role.kubernetes.io/master",
											Operator: corev1.TolerationOpExists,
										},
									}),
								),
							},
							DaemonSets: []*appsv1.DaemonSet{
								test.MakeFakeDaemonSet("busybox-ds", "simple", "500m", "512Mi",
									test.WithDaemonSetNodeSelector(map[string]string{
										"beta.kubernetes.io/os": "linux",
									}),
									test.WithDaemonSetAffinity(&corev1.Affinity{
										NodeAffinity: &corev1.NodeAffinity{
											RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
												NodeSelectorTerms: []corev1.NodeSelectorTerm{
													{
														MatchExpressions: []corev1.NodeSelectorRequirement{
															{
																Key:      "node-role.kubernetes.io/master",
																Operator: corev1.NodeSelectorOpDoesNotExist,
															},
														},
													},
												},
											},
										},
									}),
								),
							},
							Jobs: []*batchv1.Job{
								test.MakeFakeJob("pi", "default", 1, "100m", "100Mi"),
							},
							Pods: []*corev1.Pod{
								test.MakeFakePod("single-pod", "simple", "100m", "100Mi",
									test.WithPodNodeSelector(map[string]string{
										"node-role.kubernetes.io/master": "",
									}),
									test.WithPodTolerations([]corev1.Toleration{
										{
											Effect:   corev1.TaintEffectNoSchedule,
											Key:      "node-role.kubernetes.io/master",
											Operator: corev1.TolerationOpExists,
										},
									}),
								),
							},
							StatefulSets: []*appsv1.StatefulSet{
								test.MakeFakeStatefulSet("busybox-sts", "simple", 4, "1", "512Mi",
									test.WithStatefulSetTolerations([]corev1.Toleration{
										{
											Effect:   corev1.TaintEffectNoSchedule,
											Key:      "node-role.kubernetes.io/master",
											Operator: corev1.TolerationOpExists,
										},
									}),
									test.WithStatefulSetAffinity(&corev1.Affinity{
										PodAntiAffinity: &corev1.PodAntiAffinity{
											PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
												{
													Weight: 100,
													PodAffinityTerm: corev1.PodAffinityTerm{
														LabelSelector: &metav1.LabelSelector{
															MatchExpressions: []metav1.LabelSelectorRequirement{
																{
																	Key:      "app",
																	Operator: metav1.LabelSelectorOpIn,
																	Values:   []string{"busybox-sts"},
																},
															},
														},
														TopologyKey: "kubernetes.io/hostname",
													},
												},
											},
										},
									}),
								),
							},
							ReplicaSets: []*appsv1.ReplicaSet{
								test.MakeFakeReplicaSet("calico-kube-controllers", "kube-system", 2, "", "",
									test.WithReplicaSetTolerations([]corev1.Toleration{
										{
											Effect:   corev1.TaintEffectNoSchedule,
											Operator: corev1.TolerationOpExists,
										},
										{
											Key:      "CriticalAddonsOnly",
											Operator: corev1.TolerationOpExists,
										},
										{
											Effect:   corev1.TaintEffectNoExecute,
											Operator: corev1.TolerationOpExists,
										},
									})),
							},
						},
					},
				},
			},
			wantErr:       false,
			failedPodsNum: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Simulate(tt.args.cluster, tt.args.apps, tt.args.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("Simulate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if fit, failedReason := checkResult(tt.args, got, tt.failedPodsNum); !fit {
				t.Errorf("Simulate() check result failed, reason: %s", failedReason)
			}
		})
	}
}

func checkResult(args args, got *SimulateResult, failedPodsNum int64) (bool, string) {
	// check number of failed pods
	if failedPodsNum != int64(len(got.UnscheduledPods)) {
		return false, fmt.Sprintf("failedPodsNum: %d, got.UnscheduledPods: %d, got.UnscheduledPods: %v", failedPodsNum, len(got.UnscheduledPods), got.UnscheduledPods)
	}

	// get all pods
	var allPods []*corev1.Pod
	for _, nodeStatus := range got.NodeStatus {
		for _, pod := range nodeStatus.Pods {
			newPod := pod.DeepCopy()
			allPods = append(allPods, newPod)
		}
	}
	for _, pod := range got.UnscheduledPods {
		newPod := pod.Pod.DeepCopy()
		allPods = append(allPods, newPod)
	}

	// get all node
	var allNodes []*corev1.Node
	for _, node := range args.cluster.Nodes {
		newNode := node.DeepCopy()
		allNodes = append(allNodes, newNode)
	}

	// pods, including static pods and pods created by users
	individualPodNum := 0
	rstIndividualPodNum := 0
	var pods []*corev1.Pod
	pods = append(pods, args.cluster.Pods...)
	for _, app := range args.apps {
		pods = append(pods, app.Resource.Pods...)
	}
	individualPodNum = len(pods)

	// workload
	var podWorkloadMap map[workload]int64 = make(map[workload]int64)
	var rstPodWorkloadMap map[workload]int64 = make(map[workload]int64)
	// deployments
	var deployments []*appsv1.Deployment
	deployments = append(deployments, args.cluster.Deployments...)
	for _, app := range args.apps {
		deployments = append(deployments, app.Resource.Deployments...)
	}
	if len(deployments) != 0 {
		for _, deploy := range deployments {
			workload := workload{
				name:      deploy.Name,
				namespace: deploy.Namespace,
				kind:      simontype.Deployment,
			}
			podWorkloadMap[workload] = int64(*deploy.Spec.Replicas)
			rstPodWorkloadMap[workload] = 0
		}
	}

	// replicasets
	var replicasets []*appsv1.ReplicaSet
	replicasets = append(replicasets, args.cluster.ReplicaSets...)
	for _, app := range args.apps {
		replicasets = append(replicasets, app.Resource.ReplicaSets...)
	}
	if len(replicasets) != 0 {
		for _, replicaset := range replicasets {
			workload := workload{
				name:      replicaset.Name,
				namespace: replicaset.Namespace,
				kind:      simontype.ReplicaSet,
			}
			podWorkloadMap[workload] = int64(*replicaset.Spec.Replicas)
			rstPodWorkloadMap[workload] = 0
		}
	}

	// statefulsets
	var statefulsets []*appsv1.StatefulSet
	statefulsets = append(statefulsets, args.cluster.StatefulSets...)
	for _, app := range args.apps {
		statefulsets = append(statefulsets, app.Resource.StatefulSets...)
	}
	if len(statefulsets) != 0 {
		for _, sts := range statefulsets {
			workload := workload{
				name:      sts.Name,
				namespace: sts.Namespace,
				kind:      simontype.StatefulSet,
			}
			podWorkloadMap[workload] = int64(*sts.Spec.Replicas)
			rstPodWorkloadMap[workload] = 0
		}
	}

	// daemonsets
	var daemonsets []*appsv1.DaemonSet
	daemonsets = append(daemonsets, args.cluster.DaemonSets...)
	for _, app := range args.apps {
		daemonsets = append(daemonsets, app.Resource.DaemonSets...)
	}
	if len(daemonsets) != 0 {
		for _, ds := range daemonsets {
			workload := workload{
				name:      ds.Name,
				namespace: ds.Namespace,
				kind:      simontype.DaemonSet,
			}
			podWorkloadMap[workload] = 0
			rstPodWorkloadMap[workload] = 0
			for _, node := range allNodes {
				pod, _ := utils.NewDaemonPod(ds, node.Name)
				shouldRun := utils.NodeShouldRunPod(node, pod)
				if shouldRun {
					podWorkloadMap[workload] += 1
				}
			}
		}
	}

	// job
	var jobs []*batchv1.Job
	jobs = append(jobs, args.cluster.Jobs...)
	for _, app := range args.apps {
		jobs = append(jobs, app.Resource.Jobs...)
	}
	if len(jobs) != 0 {
		for _, job := range jobs {
			workload := workload{
				name:      job.Name,
				namespace: job.Namespace,
				kind:      simontype.Job,
			}
			podWorkloadMap[workload] = int64(*job.Spec.Completions)
			rstPodWorkloadMap[workload] = 0
		}
	}

	// cronjob
	var cronjobs []*batchv1beta1.CronJob
	cronjobs = append(cronjobs, args.cluster.CronJobs...)
	for _, app := range args.apps {
		cronjobs = append(cronjobs, app.Resource.CronJobs...)
	}
	if len(cronjobs) != 0 {
		for _, cronjob := range cronjobs {
			workload := workload{
				name:      cronjob.Name,
				namespace: cronjob.Namespace,
				kind:      simontype.CronJob,
			}
			podWorkloadMap[workload] = int64(*cronjob.Spec.JobTemplate.Spec.Completions)
			rstPodWorkloadMap[workload] = 0
		}
	}

	// check
	for _, pod := range allPods {
		if pod.OwnerReferences != nil {
			for _, ref := range pod.OwnerReferences {
				if ref.Kind == simontype.ReplicaSet {
					replicaset := workload{
						name:      ref.Name,
						namespace: pod.Namespace,
						kind:      simontype.ReplicaSet,
					}
					if _, exist := rstPodWorkloadMap[replicaset]; exist {
						// replicaset
						rstPodWorkloadMap[replicaset] += 1
					} else {
						// deployment
						index := strings.LastIndex(ref.Name, "-")
						deployName := ref.Name[:index]
						deployment := workload{
							name:      deployName,
							namespace: pod.Namespace,
							kind:      simontype.Deployment,
						}
						rstPodWorkloadMap[deployment] += 1
					}
				} else if ref.Kind == simontype.Job {
					job := workload{
						name:      ref.Name,
						namespace: pod.Namespace,
						kind:      simontype.Job,
					}
					if _, exist := rstPodWorkloadMap[job]; exist {
						// job
						rstPodWorkloadMap[job] += 1
					} else {
						// cronjob
						index := strings.LastIndex(ref.Name, "-")
						cronjobName := ref.Name[:index]
						cronjob := workload{
							name:      cronjobName,
							namespace: pod.Namespace,
							kind:      simontype.CronJob,
						}
						rstPodWorkloadMap[cronjob] += 1
					}
				} else if ref.Kind == simontype.StatefulSet || ref.Kind == simontype.DaemonSet {
					// statefulset
					// daemonset
					workload := workload{
						name:      ref.Name,
						namespace: pod.Namespace,
						kind:      ref.Kind,
					}
					rstPodWorkloadMap[workload] += 1
				}
			}
		} else {
			// static pods or pods created by users
			rstIndividualPodNum++
		}
	}

	// check workloads
	eq := reflect.DeepEqual(podWorkloadMap, rstPodWorkloadMap)
	if !eq {
		return false, "podWorkloadMap and rstPodWorkloadMap are not equal"
	}

	// check static pods
	if individualPodNum != rstIndividualPodNum {
		return false, "podNum and rstPodNum are not equal"
	}

	return true, ""
}
