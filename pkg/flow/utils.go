/*
Copyright 2015 The Kubernetes Authors.

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
	"k8s.io/client-go/kubernetes"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Represent tc chaos information using json encoding
type ChaosInfo struct {
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
func SetPodChaosUpdated(podAnnotations map[string]string) (newAnnotations map[string]string) {
	newAnnotations = podAnnotations
	newAnnotations["chaos-done"] = "yes"
	return newAnnotations
}

// Extract Chaos settings from pod's annotation
func ExtractPodChaosInfo(podAnnotations map[string]string) (ingressChaosInfo, egressChaosInfo ChaosInfo, needUpdate bool, err error) {
	done, found := podAnnotations["kubernetes.io/done-chaos"]
	if (found && done == "yes")||!found {
		return ingressChaosInfo, egressChaosInfo, false, nil
	}

	ingress, found := podAnnotations["kubernetes.io/ingress-chaos"]
	if found {
		json.Unmarshal([]byte(ingress), &ingressChaosInfo)
	}

	egress, found := podAnnotations["kubernetes.io/egress-chaos"]
	if found {
		json.Unmarshal([]byte(egress), &egressChaosInfo)
	}

	return ingressChaosInfo, egressChaosInfo, true, nil
}

func GetMasterIP(clientset *kubernetes.Clientset) (masterIP string){
	nodes,_:=clientset.CoreV1().Nodes().List(meta_v1.ListOptions{LabelSelector:"node-role.kubernetes.io/master="})
	masterAddrs:=nodes.Items[0].Status.Addresses

	for _, addr:=range masterAddrs{
		if addr.Type=="InternalIP"{
			masterIP=addr.Address
		}
	}
	return masterIP
}