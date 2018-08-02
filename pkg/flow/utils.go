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
)

// Represent tc chaos information using json encoding
type TCChaosInfo struct {
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
func ExtractPodChaosInfo(podAnnotations map[string]string) (ingressChaosInfo, egressChaosInfo string, tcChaosInfo TCChaosInfo, needUpdate bool, err error) {
	done, found := podAnnotations["chaos-done"]
	if (found && done == "yes")||!found {
		return "", "", tcChaosInfo, false, nil
	}

	info, found := podAnnotations["TC-chaos"]
	if found {
		json.Unmarshal([]byte(info), &tcChaosInfo)
	}

	ingressChaosInfo, found = podAnnotations["kubernetes.io/ingress-chaos"]
	if found {
	}

	egressChaosInfo, found = podAnnotations["kubernetes.io/egress-chaos"]
	if found {
	}

	return ingressChaosInfo, egressChaosInfo, tcChaosInfo, true, nil
}
