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
	"encoding/hex"
	"fmt"
	"github.com/huanwei/kube-chaos/pkg/sets"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"net"
	"strings"
)

// Change chaos-done flag to yes
func SetPodChaosUpdated(ingressNeedUpdate, egressNeedUpdate, ingressNeedClear, egressNeedClear bool, podAnnotations map[string]string) (newAnnotations map[string]string) {
	if ingressNeedUpdate {
		podAnnotations["kubernetes.io/done-ingress-chaos"] = "yes"
	}
	if egressNeedUpdate {
		podAnnotations["kubernetes.io/done-egress-chaos"] = "yes"
	}

	if ingressNeedClear {
		delete(podAnnotations, "kubernetes.io/clear-ingress-chaos")
		delete(podAnnotations, "kubernetes.io/done-ingress-chaos")
		delete(podAnnotations, "kubernetes.io/ingress-chaos")
	}
	if egressNeedClear {
		delete(podAnnotations, "kubernetes.io/clear-egress-chaos")
		delete(podAnnotations, "kubernetes.io/done-egress-chaos")
		delete(podAnnotations, "kubernetes.io/egress-chaos")
	}
	return podAnnotations
}

func GetClearFlag(podAnnotations map[string]string) (ingressNeedClear, egressNeedClear bool) {
	_, ingressNeedClear = podAnnotations["kubernetes.io/clear-ingress-chaos"]
	_, egressNeedClear = podAnnotations["kubernetes.io/clear-egress-chaos"]

	return ingressNeedClear, egressNeedClear
}

// Extract Chaos settings from pod's annotation
func ExtractPodChaosInfo(podAnnotations map[string]string) (ingressChaosInfo, egressChaosInfo string, ingressNeedUpdate, egressNeedUpdate bool, err error) {
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

	ingressChaosInfo, _ = podAnnotations["kubernetes.io/ingress-chaos"]
	egressChaosInfo, _ = podAnnotations["kubernetes.io/egress-chaos"]

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

func sliceToSets(slice []string) sets.String {
	ss := sets.String{}
	for _, s := range slice {
		ss.Insert(s)
	}
	return ss
}

// Convert a CIDR from hex representation to text, opposite of the above.
func asciiCIDR(cidr string) (string, error) {
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("unexpected CIDR format: %s", cidr)
	}
	ipData, err := hex.DecodeString(parts[0])
	if err != nil {
		return "", err
	}
	ip := net.IP(ipData)

	maskData, err := hex.DecodeString(parts[1])
	mask := net.IPMask(maskData)
	size, _ := mask.Size()

	return fmt.Sprintf("%s/%d", ip.String(), size), nil
}

// Convert a CIDR from text to a hex representation
// Strips any masked parts of the IP, so 1.2.3.4/16 becomes hex(1.2.0.0)/ffffffff
func hexCIDR(cidr string) (string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}
	ip = ip.Mask(ipnet.Mask)
	hexIP := hex.EncodeToString([]byte(ip.To4()))
	hexMask := ipnet.Mask.String()
	return hexIP + "/" + hexMask, nil
}
