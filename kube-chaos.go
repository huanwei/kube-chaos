/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Note: the example only works with the code within the same release/branch.
package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/huanwei/kube-chaos/pkg/calico"
	"github.com/huanwei/kube-chaos/pkg/flow"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"strings"
)

func main() {
	var (
		kubeconfig    string
		endpoint      string
		labelSelector string
		firstIFB      int
		secondIFB     int
		syncDuration  int
		shaper        flow.Shaper
	)

	flag.StringVar(&kubeconfig, "kubeconfig", "/etc/kubernetes/kubelet.conf", "absolute path to the kubeconfig file")
	flag.StringVar(&endpoint, "etcd-endpoint", "", "the calico etcd endpoint, use standalone etcd cluster, if not set we use the default in-cluster Calico etcd. e.g. http://10.96.232.136:6666")
	flag.StringVar(&labelSelector, "labelSelector", "chaos=on", "select pods to do chaos, e.g. chaos=on")
	flag.IntVar(&firstIFB, "firstIFB", 0, "first available ifb, default 0 e.g. 2")
	flag.IntVar(&secondIFB, "secondIFB", 1, "second available ifb, default 1 e.g. 4")
	flag.IntVar(&syncDuration, "syncDuration", 1, "sync duration(seconds)")
	flag.Parse()

	// Uses the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Println(err)
		panic(err.Error())
	}

	// Creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// Get default endpoint
	if endpoint == "" {
		endpoint = flow.GetMasterIP(clientset) + ":6666"
	}
	hostname, _ := os.Hostname()

	// Init ifb module
	err = flow.InitIfbModule(firstIFB, secondIFB)
	if err != nil {
		glog.Errorf("Failed init ifb: %v", err)
	}

	glog.Flush()

	// Synchronize pods and do chaos
	for {
		//now:=time.Now()
		// Get current Node
		node, err := clientset.CoreV1().Nodes().Get(hostname, meta_v1.GetOptions{})
		if err != nil {
			glog.Errorf("Failed get node: %v", err)
		}

		// Only control current node's pods, so select pods using node name
		pods, err := clientset.CoreV1().Pods("").List(meta_v1.ListOptions{LabelSelector: labelSelector, FieldSelector: "spec.nodeName=" + hostname})
		if err != nil {
			glog.Errorf("Failed list pods: %v", err)
		}
		glog.V(4).Infof("There are %d pods need to do chaos in the cluster\n", len(pods.Items))

		// Used for checking which tc class isn't used, and del it
		egressPodsCIDRs := []string{}
		ingressPodsCIDRs := []string{}

		// Check Node's clear flag, if it exists, clear all settings and close
		_, clearNode := node.Annotations["kubernetes.io/clear-chaos"]
		if clearNode {
			// First close the ifb of node
			glog.Info("Closing chaos...")
			err := flow.ClearIfb(firstIFB, secondIFB)
			if err != nil {
				glog.Error(err)
			}

			// Then clean each pod
			for _, pod := range pods.Items {
				// Get network card name
				workload := calico.GetWorkload(pod.Namespace, pod.Spec.NodeName, pod.Name, endpoint)

				// Clear network card settings
				err = flow.ClearIngressMirroring(workload.Spec.InterfaceName)
				if err != nil {
					glog.Errorf("Fail to clear pod %s's ingress settings: %s", pod.Name, err)
				}
				err = flow.ClearEgressMirroring(workload.Spec.InterfaceName)
				if err != nil {
					glog.Errorf("Fail to clear pod %s's egress settings: %s", pod.Name, err)
				}

				// Delete Pod flag
				pod.SetAnnotations(flow.SetPodChaosUpdated(false, false, true, true, pod.Annotations))
				clientset.CoreV1().Pods(pod.Namespace).UpdateStatus(pod.DeepCopy())

				glog.Infof("Pod %s cleared", pod.Name)
			}

			// Force update log
			glog.Infof("Closing complete")
			glog.Flush()

			// Clear Node's annotation and label
			annotations := node.Annotations
			delete(annotations, "kubernetes.io/clear-chaos")
			labels := node.Labels
			delete(labels, strings.Split(labelSelector, "=")[0])
			node.SetAnnotations(annotations)
			node.SetLabels(labels)
			clientset.CoreV1().Nodes().UpdateStatus(node.DeepCopy())

			// After label removed, k8s will delete kube-chaos from the node
			// Wait for terminating
			for {
				time.Sleep(time.Duration(syncDuration) * time.Second)
			}
		}

		// clear flag isn't exists, do chaos on all labeled pods
		for _, pod := range pods.Items {
			// Extract chaosInfo from pod's annotation
			ingressChaosInfo, egressChaosInfo, ingressNeedUpdate, egressNeedUpdate, err := flow.ExtractPodChaosInfo(pod.Annotations)
			if err != nil {
				glog.Errorf("Failed extract pod's chaos info: %v", err)
			}

			// Store the cidr of the pod
			cidr := fmt.Sprintf("%s/32", pod.Status.PodIP) //192.168.0.10/32
			egressPodsCIDRs = append(egressPodsCIDRs, cidr)
			ingressPodsCIDRs = append(ingressPodsCIDRs, cidr)

			// Neither ingress nor egress need update, skip
			if !ingressNeedUpdate && !egressNeedUpdate {
				//glog.Infof("pod %s's setting has deployed, skip", pod.Name)
				continue
			}

			// Get pod clear flag
			ingressNeedClear, egressNeedClear := flow.GetClearFlag(pod.Annotations)

			// Get pod's veth interface name
			workload := calico.GetWorkload(pod.Namespace, pod.Spec.NodeName, pod.Name, endpoint)

			// Create a shaper
			shaper = flow.NewTCShaper(workload.Spec.InterfaceName, firstIFB, secondIFB)

			if ingressNeedUpdate {
				if !ingressNeedClear {
					// Create ingress mirroring
					if err := shaper.ReconcileIngressMirroring(cidr); err != nil {
						glog.Errorf("Failed to mirror veth(%s) to ifb1: %v", workload.Spec.InterfaceName, err)
					}

					// First clear interface
					shaper.ClearIngressInterface()

					// Config pod interface  qdisc
					if err := shaper.ReconcileIngressInterface(); err != nil {
						glog.Errorf("Failed to init veth(%s): %v", workload.Spec.InterfaceName, err)
					}

					if err := shaper.ReconcileIngressCIDR(cidr, ingressChaosInfo); err != nil {
						glog.Errorf("Failed to reconcile CIDR %s: %v", cidr, err)
					}
					glog.V(4).Infof("reconcile cidr %s with ingressChaosInfo %s ", cidr, ingressChaosInfo)

					// Execute tc command in ingress
					shaper.ExecTcChaos(true, ingressChaosInfo)
				} else {
					// Clear ingress mirroring
					err := flow.ClearIngressMirroring(workload.Spec.InterfaceName)
					if err != nil {
						glog.Errorf("Fail to clear ingress mirroring: %s", err)
					}
					// Clear ingress ifb class
					err = flow.Reset(cidr, fmt.Sprintf("ifb%d", secondIFB))
					if err != nil {
						glog.Errorf("Fail to clear ingress ifb class: %s", err)
					}
				}
			}

			if egressNeedUpdate {
				if !egressNeedClear {
					// Create egress mirroring
					if err := shaper.ReconcileEgressMirroring(cidr); err != nil {
						glog.Errorf("Failed to mirror veth(%s) to ifb0: %v", workload.Spec.InterfaceName, err)
					}

					// First clear interface
					shaper.ClearEgressInterface()

					// Config pod interface  qdisc, and mirror to ifb
					if err := shaper.ReconcileEgressInterface(); err != nil {
						glog.Errorf("Failed to init veth(%s): %v", workload.Spec.InterfaceName, err)
					}

					if err := shaper.ReconcileEgressCIDR(cidr, egressChaosInfo); err != nil {
						glog.Errorf("Failed to reconcile CIDR %s: %v", cidr, err)
					}
					glog.V(4).Infof("reconcile cidr %s with egressChaosInfo %s ", cidr, egressChaosInfo)

					// Execute tc command in egress
					shaper.ExecTcChaos(false, egressChaosInfo)
				} else {
					// Clear egress mirroring
					err := flow.ClearEgressMirroring(workload.Spec.InterfaceName)
					if err != nil {
						glog.Errorf("Fail to clear egress mirroring: %s", err)
					}
					// Clear egress ifb class
					err = flow.Reset(cidr, fmt.Sprintf("ifb%d", firstIFB))
					if err != nil {
						glog.Errorf("Fail to clear egress ifb class: %s", err)
					}
				}

			}

			// Update chaos-done flag
			pod.SetAnnotations(flow.SetPodChaosUpdated(ingressNeedUpdate, egressNeedUpdate, ingressNeedClear, egressNeedClear, pod.Annotations))
			clientset.CoreV1().Pods(pod.Namespace).UpdateStatus(pod.DeepCopy())

		}
		// Delete chaos on pods not labeled
		if err := flow.DeleteExtraChaos(egressPodsCIDRs, ingressPodsCIDRs, firstIFB, secondIFB); err != nil {
			glog.Errorf("Failed to delete extra chaos: %v", err)
		}

		//elapsed:=time.Since(now)
		//glog.Infof("iteration time used: %v",elapsed)

		// Flush log
		glog.Flush()
		// Sleep to avoid high CPU usage
		time.Sleep(time.Duration(syncDuration) * time.Second)
	}

}
