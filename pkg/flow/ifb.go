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
	"github.com/huanwei/kube-chaos/pkg/exec"
	"strings"
	"github.com/golang/glog"
)

func InitIfbModule() error {
	e := exec.New()
	if _, err := e.Command("modprobe", "ifb").CombinedOutput(); err != nil {
		return err
	}
	glog.Infof("IFB mod up")
	if _, err := e.Command("ip", "link", "set", "dev", "ifb0", "up").CombinedOutput(); err != nil {
		return err
	}
	glog.Infof("IFB0 up")
	if _, err := e.Command("ip", "link", "set", "dev", "ifb1", "up").CombinedOutput(); err != nil {
		return err
	}
	glog.Infof("IFB1 up")
	if err := initIfb("ifb0"); err != nil {
		return err
	}
	glog.Infof("IFB0 inited")
	if err := initIfb("ifb1"); err != nil {
		return err
	}
	glog.Infof("IFB1 inited")
	return nil
}

func initIfb(ifb string) error {
	e := exec.New()

	// Check whether ifb has been initialized
	out, err := e.Command("tc", "qdisc", "show","dev", ifb).CombinedOutput();
	if err != nil {
		return err
	}

	outs := strings.Split(string(out), " ")
	// If already initialized, return
	if len(outs) >=12  && outs[0] == "qdisc" && outs[1] == "htb" && outs[2] == "1:" && outs[3] == "root" {
		glog.Infof("%s has already initialized",ifb)
		return nil
	}

	glog.Infof("%s not inited, initializing",ifb)
	// Else reset ifb
	e.Command("tc", "qdisc", "del", "dev", ifb, "root").CombinedOutput()
	if _, err := e.Command("tc", "qdisc", "add", "dev", ifb, "root", "handle", "1:", "htb", "default", "0").CombinedOutput(); err != nil {
		return err
	}
	return nil
}
