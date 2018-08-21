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

import "github.com/huanwei/kube-chaos/pkg/exec"

// tcShaper provides an implementation of the Shaper interface on Linux using the 'tc' tool.
// Uses the hierarchical token bucket queuing discipline (htb), this requires Linux 2.4.20 or newer
// or a custom kernel with that queuing discipline backported.
type tcShaper struct {
	e              exec.Interface
	iface          string
	firstIFB       string
	secondIFB      string
	ingressClassid string
	egressClassid  string
}

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
