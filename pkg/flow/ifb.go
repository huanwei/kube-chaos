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
	"errors"
	"fmt"
	"github.com/golang/glog"
	"github.com/huanwei/kube-chaos/pkg/exec"
	"strings"
)

func InitIfbModule(FirstIFB int) error {
	First := fmt.Sprintf("ifb%c", FirstIFB+'0')
	Second := fmt.Sprintf("ifb%c", FirstIFB+'1')
	e := exec.New()
	if _, err := e.Command("modprobe", "ifb").CombinedOutput(); err != nil {
		return err
	}
	glog.Infof("IFB mod up")
	if _, err := e.Command("ip", "link", "set", "dev", First, "up").CombinedOutput(); err != nil {
		return err
	}
	glog.Infof("%s up", First)
	if _, err := e.Command("ip", "link", "set", "dev", Second, "up").CombinedOutput(); err != nil {
		return err
	}
	glog.Infof("%s up", Second)
	if err := initIfb(First); err != nil {
		return err
	}
	glog.Infof("%s inited", First)
	if err := initIfb(Second); err != nil {
		return err
	}
	glog.Infof("%s inited", Second)
	return nil
}

func initIfb(ifb string) error {
	e := exec.New()

	// Check whether ifb has been initialized
	out, err := e.Command("tc", "qdisc", "show", "dev", ifb).CombinedOutput()
	if err != nil {
		return err
	}

	outs := strings.Split(string(out), " ")
	// If already initialized, return
	if len(outs) >= 12 && outs[0] == "qdisc" && outs[1] == "htb" && outs[2] == "1:" && outs[3] == "root" {
		glog.Infof("%s has already initialized", ifb)
		return nil
	}

	glog.Infof("%s not inited, initializing", ifb)
	// Else reset ifb
	e.Command("tc", "qdisc", "del", "dev", ifb, "root").CombinedOutput()
	if _, err := e.Command("tc", "qdisc", "add", "dev", ifb, "root", "handle", "1:", "htb", "default", "0").CombinedOutput(); err != nil {
		return err
	}
	return nil
}

func ClearIfb(firstIFB int) error {
	e := exec.New()

	_, err := e.Command("tc", "qdisc", "del", "dev", fmt.Sprintf("ifb%c", firstIFB+'0'), "root").CombinedOutput()
	if err != nil {
		return errors.New(fmt.Sprintf("fail to delete IFB%d's root qdisc: %s", firstIFB, err))
	}

	_, err = e.Command("tc", "qdisc", "del", "dev", fmt.Sprintf("ifb%c", firstIFB+'1'), "root").CombinedOutput()
	if err != nil {
		return errors.New(fmt.Sprintf("fail to delete IFB%d's root qdisc: %s", firstIFB+1, err))
	}

	return nil
}
