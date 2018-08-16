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

package calico

import (
	"encoding/json"
	"github.com/golang/glog"
	"github.com/huanwei/kube-chaos/pkg/exec"
	"strings"
)

type Workload struct {
	Spec struct {
		InterfaceName string
	}
}

func GetWorkload(namespace, nodeName, podName, endpoint string) Workload {
	e := exec.New()

	podNames := strings.Split(podName, "-")
	newPodName := strings.Join(podNames, "--")

	nodeNames := strings.Split(nodeName, "-")
	newNodeName := strings.Join(nodeNames, "--")

	cmd := namespace + "/" + newNodeName + "-k8s-" + newPodName + "-eth0"

	data, err := e.Command("etcdctl", "get", "--endpoints="+endpoint, "--prefix", "/calico/resources/v3/projectcalico.org/workloadendpoints/"+cmd).CombinedOutput()

	if err != nil {
		glog.Errorf("Failed fetch pod %s's interface name: %s :%s", podName, err, data)
	}

	workload := Workload{}

	err = json.Unmarshal([]byte(strings.Split(string(data), "\n")[1]), &workload)
	if err != nil {
		glog.Errorf("JSON parse error: %v", err)
	}

	glog.Infof("Interface name got: %s", workload.Spec.InterfaceName)

	return workload
}
