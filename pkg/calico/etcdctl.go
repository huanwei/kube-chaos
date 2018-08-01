package calico

import (
	"github.com/huanwei/kube-chaos/pkg/exec"
	"github.com/golang/glog"
	"encoding/json"
	"strings"
)


type Workload struct {
	Spec	struct{
		InterfaceName	string
	}
}

func GetWorkload(namespace,nodeName,podName string) Workload {
	e := exec.New()

	//data, err := e.Command("etcdctl", "get", "--endpoints=10.211.55.10:6666","--prefix",
	//	"/calico/resources/v3/projectcalico.org/workloadendpoints/"+namespace+"/"+nodeName+"-k8s-"+podName+"-eth0").CombinedOutput()

	podNames:=strings.Split(podName,"-")
	newPodName:=strings.Join(podNames,"--")


	cmd:=namespace+"/"+nodeName+"-k8s-"+newPodName+"-eth0"

	data, err := e.Command("etcdctl", "get", "--endpoints=10.211.55.10:6666","--prefix", "/calico/resources/v3/projectcalico.org/workloadendpoints/"+cmd).CombinedOutput()

	if(err!=nil){
		glog.Errorf("Failed fetch pod %s's interface name: %s :%s", podName,err,data)
	}

	workload := Workload{}

	err = json.Unmarshal([]byte(strings.Split(string(data),"\n")[1]), &workload)
	if err != nil {
		glog.Errorf("JSON parse error: %v", err)
	}

	glog.Infof("Interface name got: %s",workload.Spec.InterfaceName)

	return workload
}
