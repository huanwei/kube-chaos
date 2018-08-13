/*
Copyright 2016 The Kubernetes Authors.

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
)

func main() {
	var (
		kubeconfig    string
		endpoint      string
		labelSelector string
		firstIFB      int
		syncDuration  int
		shaper        flow.Shaper
	)

	flag.StringVar(&kubeconfig, "kubeconfig", "/etc/kubernetes/kubelet.conf", "absolute path to the kubeconfig file")
	flag.StringVar(&endpoint, "etcd-endpoint", "", "the calico etcd endpoint, e.g. http://10.96.232.136:6666")
	flag.StringVar(&labelSelector, "labelSelector", "chaos=on", "select pods to do chaos, e.g. chaos=on")
	flag.IntVar(&firstIFB, "firstIFB", 0, "first available ifb, e.g. if ifb0 and ifb1 are used, set it to 2")
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

	if endpoint == "" {
		endpoint = flow.GetMasterIP(clientset) + ":6666"
	}
	hostname, _ := os.Hostname()
	// Init ifb module
	err = flow.InitIfbModule(firstIFB)
	if err != nil {
		glog.Errorf("Failed init ifb: %v", err)
	}

	// Synchronize pods and do chaos
	for {
		//now:=time.Now()
		// Only control current node's pods, so select pods using node name
		pods, err := clientset.CoreV1().Pods("").List(meta_v1.ListOptions{LabelSelector: labelSelector, FieldSelector: "spec.nodeName=" + hostname})
		if err != nil {
			glog.Errorf("Failed list pods: %v", err)
		}
		glog.V(4).Infof("There are %d pods need to do chaos in the cluster\n", len(pods.Items))

		// Used for  checking which tc class isn't used, and del it
		egressPodsCIDRs := []string{}
		ingressPodsCIDRs := []string{}

		for _, pod := range pods.Items {

			// todo - fix
			ingressChaosInfo, egressChaosInfo, ingressNeedUpdate, egressNeedUpdate, err := flow.ExtractPodChaosInfo(pod.Annotations)
			if err != nil {
				glog.Errorf("Failed extract pod's chaos info: %v", err)
			}

			cidr := fmt.Sprintf("%s/32", pod.Status.PodIP) //192.168.0.10/32
			egressPodsCIDRs = append(egressPodsCIDRs, cidr)
			ingressPodsCIDRs = append(ingressPodsCIDRs, cidr)

			if !ingressNeedUpdate && !egressNeedUpdate {
				//glog.Infof("pod %s's setting has deployed, skip", pod.Name)
				continue
			}

			ingressNeedClear, egressNeedClear := flow.GetClearFlag(pod.Annotations)

			// Get pod's veth interface name
			workload := calico.GetWorkload(pod.Namespace, pod.Spec.NodeName, pod.Name, endpoint)

			// Create a shaper
			shaper = flow.NewTCShaper(workload.Spec.InterfaceName,firstIFB)

			if ingressNeedUpdate {

				if err := shaper.ReconcileIngressMirroring(cidr); err != nil {
					glog.Errorf("Failed to mirror veth(%s) to ifb1: %v", workload.Spec.InterfaceName, err)
				}

				// First clear interface
				shaper.ClearIngressInterface()

				if !ingressNeedClear {
					// Config pod interface  qdisc
					if err := shaper.ReconcileIngressInterface(ingressChaosInfo); err != nil {
						glog.Errorf("Failed to init veth(%s): %v", workload.Spec.InterfaceName, err)
					}

					if err := shaper.ReconcileIngressCIDR(cidr, ingressChaosInfo); err != nil {
						glog.Errorf("Failed to reconcile CIDR %s: %v", cidr, err)
					}
					glog.V(4).Infof("reconcile cidr %s with ingressChaosInfo %s ", cidr, ingressChaosInfo)

					// Execute tc command in ingress
					shaper.ExecTcChaos(true, ingressChaosInfo)
				}
			}

			if egressNeedUpdate {
				if err := shaper.ReconcileEgressMirroring(cidr); err != nil {
					glog.Errorf("Failed to mirror veth(%s) to ifb0: %v", workload.Spec.InterfaceName, err)
				}

				// First clear interface
				shaper.ClearEgressInterface()

				if !egressNeedClear {
					// Config pod interface  qdisc, and mirror to ifb
					if err := shaper.ReconcileEgressInterface(egressChaosInfo); err != nil {
						glog.Errorf("Failed to init veth(%s): %v", workload.Spec.InterfaceName, err)
					}

					if err := shaper.ReconcileEgressCIDR(cidr, egressChaosInfo); err != nil {
						glog.Errorf("Failed to reconcile CIDR %s: %v", cidr, err)
					}
					glog.V(4).Infof("reconcile cidr %s with egressChaosInfo %s ", cidr, egressChaosInfo)

					// Execute tc command in egress
					shaper.ExecTcChaos(false, egressChaosInfo)
				}

			}

			// Update chaos-done flag
			pod.SetAnnotations(flow.SetPodChaosUpdated(ingressNeedUpdate, egressNeedUpdate, ingressNeedClear, egressNeedClear, pod.Annotations))
			clientset.CoreV1().Pods(pod.Namespace).UpdateStatus(pod.DeepCopy())

		}
		if err := flow.DeleteExtraChaos(egressPodsCIDRs, ingressPodsCIDRs,firstIFB); err != nil {
			glog.Errorf("Failed to delete extra chaos: %v", err)
		}

		//elapsed:=time.Since(now)
		//glog.Infof("iteration time used: %v",elapsed)

		// Sleep to avoid high CPU usage
		time.Sleep(time.Duration(syncDuration) * time.Second)
	}

}
