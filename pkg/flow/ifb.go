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

// Initialize two ifb modules using input id
func InitIfbModule(firstIFB, secondIFB int) error {
	first := fmt.Sprintf("ifb%d", firstIFB)
	second := fmt.Sprintf("ifb%d", secondIFB)
	e := exec.New()

	// Load ifb module
	if _, err := e.Command("modprobe", "ifb").CombinedOutput(); err != nil {
		return err
	}
	glog.Infof("IFB mod up")

	// Set two ifb devices up
	if _, err := e.Command("ip", "link", "set", "dev", first, "up").CombinedOutput(); err != nil {
		return err
	}
	glog.Infof("ifb%d up", first)
	if _, err := e.Command("ip", "link", "set", "dev", second, "up").CombinedOutput(); err != nil {
		return err
	}
	glog.Infof("%s up", second)

	// Initialize two ifb interfaces' root queue discipline
	if err := initIfb(first); err != nil {
		return err
	}
	glog.Infof("ifb%d inited", first)
	if err := initIfb(second); err != nil {
		return err
	}
	glog.Infof("ifb%d inited", second)
	return nil
}

//Initialize ifb interface's root queue discipline
func initIfb(ifb string) error {
	e := exec.New()

	// Check whether ifb has been initialized
	out, err := e.Command("tc", "qdisc", "show", "dev", ifb).CombinedOutput()
	if err != nil {
		return err
	}

	outs := strings.Split(string(out), " ")
	// If it's already initialized, return
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

// Set ifb devices down and clean the root queue discipline
func ClearIfb(firstIFB, secondIFB int) error {
	e := exec.New()

	// Set ifb devices down
	if _, err := e.Command("ip", "link", "set", "dev", fmt.Sprintf("ifb%d", firstIFB), "down").CombinedOutput(); err != nil {
		return err
	}
	glog.Infof("ifb%d down", firstIFB)
	if _, err := e.Command("ip", "link", "set", "dev", fmt.Sprintf("ifb%d", secondIFB), "down").CombinedOutput(); err != nil {
		return err
	}
	glog.Infof("ifb%d down", secondIFB)

	// Clean the root queue discipline
	_, err := e.Command("tc", "qdisc", "del", "dev", fmt.Sprintf("ifb%d", firstIFB), "root").CombinedOutput()
	if err != nil {
		return errors.New(fmt.Sprintf("fail to delete ifb%d's root qdisc: %s", firstIFB, err))
	}

	_, err = e.Command("tc", "qdisc", "del", "dev", fmt.Sprintf("ifb%d", secondIFB), "root").CombinedOutput()
	if err != nil {
		return errors.New(fmt.Sprintf("fail to delete ifb%d's root qdisc: %s", secondIFB, err))
	}

	return nil
}
