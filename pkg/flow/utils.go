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

package flow

import (
	"encoding/json"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Represent tc chaos information using json encoding
type ChaosInfo struct {
	Rate  string
	Delay struct {
		Set       string
		Time      string
		Variation string
	}
	Loss struct {
		Set        string
		Percentage string
		Relate     string
	}
	Duplicate struct {
		Set        string
		Percentage string
	}
	Reorder struct {
		Set         string
		Time        string
		Percengtage string
		Relate      string
	}
	Corrupt struct {
		Set        string
		Percentage string
	}
}

// Change chaos-done flag to yes
func SetPodChaosUpdated(ingressNeedUpdate,egressNeedUpdate,ingressNeedClear, egressNeedClear bool, podAnnotations map[string]string) (newAnnotations map[string]string) {
	if ingressNeedUpdate{
		podAnnotations["kubernetes.io/done-ingress-chaos"] = "yes"
	}
	if egressNeedUpdate{
		podAnnotations["kubernetes.io/done-egress-chaos"] = "yes"
	}

	if ingressNeedClear {
		delete(podAnnotations,"kubernetes.io/clear-ingress-chaos")
		delete(podAnnotations,"kubernetes.io/done-ingress-chaos")
		delete(podAnnotations,"kubernetes.io/ingress-chaos")
	}
	if egressNeedClear {
		delete(podAnnotations,"kubernetes.io/clear-egress-chaos")
		delete(podAnnotations,"kubernetes.io/done-egress-chaos")
		delete(podAnnotations,"kubernetes.io/egress-chaos")
	}
	return podAnnotations
}

func GetClearFlag(podAnnotations map[string]string) (ingressNeedClear, egressNeedClear bool) {
	_, ingressNeedClear = podAnnotations["kubernetes.io/clear-ingress-chaos"]
	_, egressNeedClear = podAnnotations["kubernetes.io/clear-egress-chaos"]

	return ingressNeedClear, egressNeedClear
}

// Extract Chaos settings from pod's annotation
func ExtractPodChaosInfo(podAnnotations map[string]string) (ingressChaosInfo, egressChaosInfo ChaosInfo, ingressNeedUpdate, egressNeedUpdate bool, err error) {
	ingressDone, found := podAnnotations["kubernetes.io/done-ingress-chaos"]
	if (found && ingressDone == "yes") || !found {
		ingressNeedUpdate = false
	} else {
		ingressNeedUpdate = true
	}

	egressDone, found := podAnnotations["kubernetes.io/done-egress-chaos"]
	if (found && egressDone == "yes") || !found {
		egressNeedUpdate = false
	} else {
		egressNeedUpdate = true
	}

	ingress, found := podAnnotations["kubernetes.io/ingress-chaos"]
	if found {
		json.Unmarshal([]byte(ingress), &ingressChaosInfo)
	}

	egress, found := podAnnotations["kubernetes.io/egress-chaos"]
	if found {
		json.Unmarshal([]byte(egress), &egressChaosInfo)
	}

	return ingressChaosInfo, egressChaosInfo, ingressNeedUpdate, egressNeedUpdate, nil
}

func GetMasterIP(clientset *kubernetes.Clientset) (masterIP string) {
	nodes, _ := clientset.CoreV1().Nodes().List(meta_v1.ListOptions{LabelSelector: "node-role.kubernetes.io/master="})
	masterAddrs := nodes.Items[0].Status.Addresses

	for _, addr := range masterAddrs {
		if addr.Type == "InternalIP" {
			masterIP = addr.Address
		}
	}
	return masterIP
}
