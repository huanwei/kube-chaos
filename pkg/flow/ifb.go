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
	"bufio"
	"bytes"
	"github.com/huanwei/kube-chaos/pkg/exec"
	"strings"
)

func InitIfbModule() error {
	e := exec.New()
	if _, err := e.Command("modprobe", "ifb").CombinedOutput(); err != nil {
		return err
	}
	if _, err := e.Command("ip", "link", "set", "dev", "ifb0", "up").CombinedOutput(); err != nil {
		return err
	}
	if _, err := e.Command("ip", "link", "set", "dev", "ifb1", "up").CombinedOutput(); err != nil {
		return err
	}
	if err := initIfb("ifb0"); err != nil {
		return err
	}
	if err := initIfb("ifb1"); err != nil {
		return err
	}
	return nil
}

/*
[root@localhost ~]# tc qdisc show dev ifb0
qdisc pfifo_fast 0: root refcnt 2 bands 3 priomap  1 2 2 2 1 2 0 0 1 1 1 1 1 1 1 1
[root@localhost ~]# tc qdisc add dev ifb0 root handle 1: htb default 30
[root@localhost ~]# tc qdisc show dev ifb0
qdisc htb 1: root refcnt 2 r2q 10 default 30 direct_packets_stat 0
*/
func initIfb(ifb string) error {
	e := exec.New()
	data, err := e.Command("tc", "qdisc", "show", "dev", ifb).CombinedOutput()
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	spec := "htb 1:"
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}
		if strings.Contains(line, spec) {
			return nil
		}
	}
	if _, err := e.Command("tc", "qdisc", "add", "dev", ifb, "root", "handle", "1:", "htb", "default", "30").CombinedOutput(); err != nil {
		return err
	}
	return nil
}
