//// +build linux

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
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"github.com/huanwei/kube-chaos/pkg/exec"
	"github.com/huanwei/kube-chaos/pkg/sets"

	"errors"
	"github.com/golang/glog"
)

// Create a new shaper
func NewTCShaper(iface string, firstIFB, secondIFB int) Shaper {
	shaper := &tcShaper{
		e:         exec.New(),
		iface:     iface,
		firstIFB:  fmt.Sprintf("ifb%d", firstIFB),
		secondIFB: fmt.Sprintf("ifb%d", secondIFB),
	}
	return shaper
}

// Execute command and log
func (t *tcShaper) execAndLog(cmdStr string, args ...string) error {
	glog.V(4).Infof("Running: %s %s", cmdStr, strings.Join(args, " "))
	cmd := t.e.Command(cmdStr, args...)
	out, err := cmd.CombinedOutput()
	glog.V(4).Infof("Output from tc: %s", string(out))
	return err
}

// Find available class id in ifb
func (t *tcShaper) nextClassID(ifb string) (int, error) {
	// Use tc command to get used class id on device
	data, err := t.e.Command("tc", "class", "show", "dev", ifb).CombinedOutput()
	if err != nil {
		return -1, err
	}

	// Scan the output
	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	classes := sets.String{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// skip empty lines
		if len(line) == 0 {
			continue
		}
		parts := strings.Split(line, " ")

		if len(parts) != 14 && len(parts) != 16 {
			return -1, fmt.Errorf("unexpected output from tc: %s (%v)", scanner.Text(), parts)
		}
		// Store used class id
		classes.Insert(parts[2])
	}

	// Make sure it doesn't go forever
	for nextClass := 1; nextClass < 10000; nextClass++ {
		if !classes.Has(fmt.Sprintf("1:%d", nextClass)) {
			return nextClass, nil
		}
	}
	// This should really never happen
	return -1, fmt.Errorf("exhausted class space, please try again")
}

// Find class using handle
func findCIDRClass(cidr, ifb string) (class, handle string, found bool, err error) {
	// Show all tc filters on device
	e := exec.New()
	data, err := e.Command("tc", "filter", "show", "dev", ifb).CombinedOutput()
	if err != nil {
		return "", "", false, err
	}

	// Convert cidr from dot-decimal to hexadecimal
	hex, err := hexCIDR(cidr)
	if err != nil {
		return "", "", false, err
	}
	spec := fmt.Sprintf("match %s", hex)

	// Scan the output
	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	filter := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}
		// Find keyword "filter", go to the next line to get class id
		if strings.HasPrefix(line, "filter") {
			filter = line
			continue
		}
		// Extract target class id and filter's handle id
		if strings.Contains(line, spec) {
			parts := strings.Split(filter, " ")
			if len(parts) != 19 {
				return "", "", false, fmt.Errorf("unexpected output from tc: %s %d (%v)", filter, len(parts), parts)
			}
			return parts[18], parts[9], true, nil
		}
	}
	return "", "", false, nil
}

// Check whether the corresponding class exists
func (t *tcShaper) classExists(classid, ifb string) (bool, error) {
	// Get existed classes on device
	data, err := t.e.Command("tc", "class", "show", "dev", ifb).CombinedOutput()
	if err != nil {
		return false, err
	}

	// Scan the output
	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	classFound := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// skip empty lines
		if len(line) == 0 {
			continue
		}
		parts := strings.Split(line, " ")
		// Expected:
		// class htb 1:1 root leaf 99f9: prio 0 rate 800000bit ceil 800000bit burst 1600b cburst 1600b
		if parts[2] == classid {
			classFound = true
			glog.Infof("Find class %s at %s was already added", classid, ifb)
			break
		}
	}
	return classFound, nil
}

// Create a new class in ifb with given class id and rate limitation
func (t *tcShaper) makeNewClass(rate, ifb string, class int) error {
	if err := t.execAndLog("tc", "class", "add",
		"dev", ifb,
		"parent", "1:",
		"classid", fmt.Sprintf("1:%d", class),
		"htb", "rate", rate); err != nil {
		return err
	}
	return nil
}

// Change class of given id in ifb with new rate limitation
func (t *tcShaper) changeClass(rate, ifb string, classid string) error {
	if err := t.execAndLog("tc", "class", "change",
		"dev", ifb,
		"parent", "1:",
		"classid", classid,
		"htb", "rate", rate); err != nil {
		return err
	}
	return nil
}

// tests to see if an interface exists, if it does, return true and the status line for the interface
// returns false, "", <err> if an error occurs.
func (t *tcShaper) qdiscExists(vethName string) (bool, bool, error) {
	data, err := t.e.Command("tc", "qdisc", "show", "dev", vethName).CombinedOutput()
	if err != nil {
		return false, false, err
	}
	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	spec1 := "htb 1: root"
	spec2 := "ingress ffff:"
	rootQdisc := false
	ingressQdisc := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}
		if strings.Contains(line, spec1) {
			rootQdisc = true
		}
		if strings.Contains(line, spec2) {
			ingressQdisc = true
		}
	}
	return rootQdisc, ingressQdisc, nil
}

func (t *tcShaper) ReconcileIngressCIDR(cidr string, ingressChaosInfo string) error {
	glog.V(4).Infof("Shaper CIDR %s with ingressChaosInfo %s", cidr, ingressChaosInfo)
	return nil
}

func (t *tcShaper) ReconcileEgressCIDR(cidr string, egressChaosInfo string) error {
	glog.V(4).Infof("Shaper CIDR %s with egressChaosInfo %s", cidr, egressChaosInfo)
	return nil
}

// Add netem in ingress class
func (t *tcShaper) ReconcileIngressInterface() error {
	e := exec.New()

	// For ingress test
	data, err := e.Command("tc", "qdisc", "add", "dev", t.secondIFB, "parent",
		t.ingressClassid, "netem").CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s\n%s", err, data)
		return err
	} else {
		glog.Infof("Ingress netem added")
	}
	return nil
}

// Add netem in egress class
func (t *tcShaper) ReconcileEgressInterface() error {
	e := exec.New()

	// For egress test
	data, err := e.Command("tc", "qdisc", "add", "dev", t.firstIFB, "parent",
		t.egressClassid, "netem").CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s\n%s", err, data)
		return err
	} else {
		glog.Infof("Egress netem added")
	}
	return nil
}

// Delete netem in ingress class
func (t *tcShaper) ClearIngressInterface() error {
	e := exec.New()

	glog.Infof("Clear ingress interface of class id: " + t.ingressClassid)
	e.Command("tc", "qdisc", "del", "dev", t.secondIFB, "parent",
		t.ingressClassid).CombinedOutput()

	return nil
}

// Delete netem in egress class
func (t *tcShaper) ClearEgressInterface() error {
	e := exec.New()

	glog.Infof("Clear egress interface of class id: " + t.egressClassid)
	e.Command("tc", "qdisc", "del", "dev", t.firstIFB, "parent",
		t.egressClassid).CombinedOutput()

	return nil
}

// Delete ingress mirroring
func ClearIngressMirroring(iface string) error {
	e := exec.New()

	glog.Infof("Clear ingress mirroring")
	out, err := e.Command("tc", "qdisc", "del", "dev", iface, "root").CombinedOutput()
	if err != nil {
		return errors.New(fmt.Sprintf("fail to delete %s's ingress mirroring: %s\n%s", iface, err, out))
	}

	return nil
}

// Delete egress mirroring
func ClearEgressMirroring(iface string) error {
	e := exec.New()

	glog.Infof("Clear egress mirroring")
	out, err := e.Command("tc", "qdisc", "del", "dev", iface, "ingress").CombinedOutput()
	if err != nil {
		return errors.New(fmt.Sprintf("fail to delete %s's egress mirroring: %s\n%s", iface, err, out))
	}

	return nil
}

// Create ingress mirroring without breaking the existing one
func (t *tcShaper) ReconcileIngressMirroring(cidr string) error {
	e := exec.New()

	// Tested highest settable rate on tc
	rate := "4gbps"
	// Tested queue size
	size := "1600"

	class, _, isFind, err := findCIDRClass(cidr, t.secondIFB)
	if err != nil {
		glog.Errorf("Error when finding class id: %s", err)
		return err
	}

	isExist := false
	if isFind {
		isExist, err = t.classExists(class, t.secondIFB)
		if err != nil {
			glog.Errorf("Error when checking class id existence: %s", err)
			return err
		}
		if !isExist {
			// Class not exist but filter was added, delete the useless filter
			// tc filter del dev SecondIFB parent 1:
			glog.Infof("Deleting useless filter at %s", t.secondIFB)
			data, err := e.Command("tc", "filter", "del", "dev", t.secondIFB, "parent",
				"1:").CombinedOutput()
			if err != nil {
				glog.Errorf("TC exec error: %s\n%s", err, data)
				return err
			} else {
				glog.Infof("filter deleted")
			}
		}
	}

	if isFind && isExist {
		glog.Infof("%s has already been initialized", t.secondIFB)
		t.ingressClassid = class
	} else {
		// Clear the root queue of the interface
		e.Command("tc", "qdisc", "del", "dev", t.iface, "root").CombinedOutput()
		glog.Infof("Clear ingress interface: %s", t.iface)

		// Add htb queue at the root of the interface
		glog.Infof("Adding htb to interface: %s", t.iface)
		data, err := e.Command("tc", "qdisc", "add", "dev", t.iface, "root",
			"handle", "1:", "htb", "default", "1").CombinedOutput()
		if err != nil {
			glog.Errorf("TC exec error: %s\n%s", err, data)
			return err
		} else {
			glog.Infof("HTB on root added")
		}

		// Add htb class
		data, err = e.Command("tc", "class", "add", "dev", t.iface,
			"parent", "1:", "classid", "1:1", "htb", "rate", rate).CombinedOutput()
		if err != nil {
			glog.Errorf("TC exec error: %s\n%s", err, data)
			return err
		} else {
			glog.Infof("HTB class 1 added")
		}

		// Add pfifo queue after the class
		data, err = e.Command("tc", "qdisc", "add", "dev", t.iface, "parent", "1:1",
			"handle", "2:1", "pfifo", "limit", size).CombinedOutput()
		if err != nil {
			glog.Errorf("TC exec error: %s\n%s", err, data)
			return err
		} else {
			glog.Infof("pfifo queue added at root")
		}

		// Mirror the egress of caliXXX to SecondIFB
		data, err = e.Command("tc", "filter", "add", "dev", t.iface, "parent", "1:", "protocol", "ip",
			"prio", "1", "u32", "match", "u32", "0", "0", "flowid", "1:1",
			"action", "mirred", "egress", "redirect", "dev", t.secondIFB).CombinedOutput()
		if err != nil {
			glog.Errorf("TC exec error: %s\n%s", err, data)
			return err
		} else {
			glog.Infof("Egress of %s mirrored to %s", t.iface, t.secondIFB)
		}

		// Get an unused classid
		classid, err := t.nextClassID(t.secondIFB)
		if err != nil {
			return err
		} else {
			t.ingressClassid = fmt.Sprintf("1:%d", classid)
			glog.Infof("%s get class %s", t.secondIFB, t.ingressClassid)
		}

		// Add a filter
		data, err = e.Command("tc", "filter", "add", "dev", t.secondIFB, "parent", "1:0", "protocol", "ip",
			"prio", "1", "u32", "match", "ip", "dst", cidr, "flowid", t.ingressClassid,
		).CombinedOutput()
		if err != nil {
			glog.Errorf("TC exec error: %s\n%s", err, data)
			return err
		} else {
			glog.Infof("Filter added")
		}

		// Create a class at SecondIFB
		err = t.makeNewClass(rate, t.secondIFB, classid)
		if err != nil {
			glog.Errorf("TC exec error: %s\n", err)
			return err
		} else {
			glog.Infof("%s class added", t.secondIFB)
		}
	}

	return nil
}

// Create egress mirroring without breaking the existing one
func (t *tcShaper) ReconcileEgressMirroring(cidr string) error {
	e := exec.New()

	// Tested highest settable rate on tc
	rate := "4gbps"

	class, _, isFind, err := findCIDRClass(cidr, t.firstIFB)
	if err != nil {
		glog.Errorf("Error when finding class id: %s", err)
		return err
	}

	isExist := false
	if isFind {
		isExist, err = t.classExists(class, t.firstIFB)
		if err != nil {
			glog.Errorf("Error when checking class id existence: %s", err)
			return err
		}
		if !isExist {
			// Class not exist but filter was added, delete the useless filter
			// tc filter del dev FirstIFB parent 1:
			glog.Infof("Deleting useless filter at %s", t.firstIFB)
			data, err := e.Command("tc", "filter", "del", "dev", t.firstIFB, "parent",
				"1:").CombinedOutput()
			if err != nil {
				glog.Errorf("TC exec error: %s\n%s", err, data)
				return err
			} else {
				glog.Infof("filter deleted")
			}
		}
	}

	if isFind && isExist {
		glog.Infof("%s has already been initialized", t.firstIFB)
		t.egressClassid = class
	} else {

		// Delete ingress queue.
		e.Command("tc", "qdisc", "del", "dev", t.iface, "ingress").CombinedOutput()

		// Add qdisc of ingress
		data, err := e.Command("tc", "qdisc", "add", "dev", t.iface, "ingress").CombinedOutput()
		if err != nil {
			glog.Errorf("TC exec error: %s\n%s", err, data)
			return err
		} else {
			glog.Infof("Ingress added")
		}

		// Mirror the ingress of caliXXX to FirstIFB
		data, err = e.Command("tc", "filter", "add", "dev", t.iface, "parent", "ffff:", "protocol", "ip",
			"prio", "1", "u32", "match", "u32", "0", "0", "flowid", "1:1",
			"action", "mirred", "egress", "redirect", "dev", t.firstIFB).CombinedOutput()
		if err != nil {
			glog.Errorf("TC exec error: %s\n%s", err, data)
			return err
		} else {
			glog.Infof("Ingress of %s mirrored to %s", t.iface, t.firstIFB)
		}

		// Get an unused classid
		classid, err := t.nextClassID(t.firstIFB)
		if err != nil {
			return err
		} else {
			t.egressClassid = fmt.Sprintf("1:%d", classid)
			glog.Infof("%s get class %s", t.firstIFB, t.egressClassid)
		}

		// Add a filter
		data, err = e.Command("tc", "filter", "add", "dev", t.firstIFB, "parent", "1:0", "protocol", "ip",
			"prio", "1", "u32", "match", "ip", "src", cidr, "flowid", t.egressClassid,
		).CombinedOutput()
		if err != nil {
			glog.Errorf("TC exec error: %s\n%s", err, data)
			return err
		} else {
			glog.Infof("Filter added")
		}

		// Create a class
		err = t.makeNewClass(rate, t.firstIFB, classid)
		if err != nil {
			glog.Errorf("TC exec error: %s\n", err)
			return err
		} else {
			glog.Infof("%s class added", t.firstIFB)
		}
	}
	return nil
}

// Limit transmission rate
func (t *tcShaper) Rate(classid, ifb string, rate string) error {
	// For test
	glog.Infof("Adding rate %s to interface: %s", rate, ifb)
	t.changeClass(rate, ifb, classid)

	return nil
}

// Add empty netem queue discipline
func (t *tcShaper) Netem(classid, ifb string, args ...string) error {
	// tc  qdisc  add  dev  eth0  root  netem
	e := exec.New()

	// For test
	glog.Infof("Adding netem %v to interface: %s", args, ifb)
	cmd := []string{"qdisc", "change", "dev", ifb, "parent", classid, "netem"}
	cmd = append(cmd, args...)

	data, err := e.Command("tc", cmd...).CombinedOutput()

	if err != nil {
		glog.Errorf("TC exec error: %s\n%s", err, data)
		return err
	} else {
		glog.Infof("Netem added")
	}

	return nil
}

// Emulate packets loss
func (t *tcShaper) Loss(classid, ifb string, args ...string) error {
	// tc  qdisc  add  dev  eth0  root  netem  loss  1%  30%
	e := exec.New()

	// For test
	glog.Infof("Adding loss %v to interface: %s", args, ifb)
	cmd := []string{"qdisc", "change", "dev", ifb, "parent", classid, "netem", "loss"}
	cmd = append(cmd, args...)

	data, err := e.Command("tc", cmd...).CombinedOutput()

	if err != nil {
		glog.Errorf("TC exec error: %s\n%s", err, data)
		return err
	} else {
		glog.Infof("Loss added")
	}

	return nil
}

// Emulate delay
func (t *tcShaper) Delay(classid, ifb string, args ...string) error {
	// tc  qdisc  add  dev  eth0  root  netem  delay  100ms  10ms  30%
	//												 basis	devi  devirate
	e := exec.New()

	// For test
	glog.Infof("Adding delay %v to interface: %s", args, ifb)
	cmd := []string{"qdisc", "change", "dev", ifb, "parent", classid, "netem", "delay"}
	cmd = append(cmd, args...)

	data, err := e.Command("tc", cmd...).CombinedOutput()

	if err != nil {
		glog.Errorf("TC exec error: %s\n%s", err, data)
		return err
	} else {
		glog.Infof("Delay added")
	}

	return nil
}

// Emulate duplicated packets
func (t *tcShaper) Duplicate(classid, ifb string, args ...string) error {
	// tc  qdisc  add  dev  eth0  root  netem  duplicate 1%
	e := exec.New()

	// For test
	glog.Infof("Adding duplicate %v to interface: %s", args, ifb)
	cmd := []string{"qdisc", "change", "dev", ifb, "parent", classid, "netem", "duplicate"}
	cmd = append(cmd, args...)

	data, err := e.Command("tc", cmd...).CombinedOutput()

	if err != nil {
		glog.Errorf("TC exec error: %s ,\n%s", err, data)
		return err
	} else {
		glog.Infof("Duplicate added")
	}

	return nil
}

// Emulate corrupted packets
func (t *tcShaper) Corrupt(classid, ifb string, args ...string) error {
	// tc  qdisc  add  dev  eth0  root  netem  corrupt  0.2%
	e := exec.New()

	// For test
	glog.Infof("Adding corrupt %v to interface: %s", args, ifb)
	cmd := []string{"qdisc", "change", "dev", ifb, "parent", classid, "netem", "corrupt"}
	cmd = append(cmd, args...)

	data, err := e.Command("tc", cmd...).CombinedOutput()

	if err != nil {
		glog.Errorf("TC exec error: %s ,\n%s", err, data)
		return err
	} else {
		glog.Infof("Corrupt added")
	}

	return nil
}

// Delete netem in the class
func (t *tcShaper) Clear(classid, ifb string, percentage, relate string) error {
	e := exec.New()
	glog.Infof("Deleting HTB in interface: %s", t.iface)
	// For test

	data, err := e.Command("tc", "qdisc", "del", "dev", ifb, "parent",
		classid, "netem").CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s\n%s", err, data)
		return err
	} else {
		glog.Infof("Netem deleted")
	}

	return nil
}

// Execute chaos settings in ingress or egress from chaosinfo
func (t *tcShaper) ExecTcChaos(isIngress bool, info string) error {
	// Split commands
	cmds := strings.Split(info, ",")

	var classid, ifb string
	if isIngress {
		classid = t.ingressClassid
		ifb = t.secondIFB
	} else {
		classid = t.egressClassid
		ifb = t.firstIFB
	}
	if info == "" {
		return errors.New("No chaos info set")
	}

	if cmds[0] == "" {
		cmds[0] = "4gbps"
	}
	err := t.Rate(classid, ifb, cmds[0])
	if err != nil {
		return err
	}

	// Set netem
	return t.Netem(classid,ifb,cmds[1:]...)
}

// Remove a bandwidth limit for a particular CIDR on a particular network interface
func Reset(cidr, ifb string) error {
	e := exec.New()
	class, handle, found, err := findCIDRClass(cidr, ifb)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("Failed to find cidr: %s on interface: %s", cidr, ifb)
	}
	glog.V(4).Infof("Delete  filter of %s on %s", cidr, ifb)
	if _, err := e.Command("tc", "filter", "del",
		"dev", ifb,
		"parent", "1:",
		"proto", "ip",
		"prio", "1",
		"handle", handle, "u32").CombinedOutput(); err != nil {
		return err
	}
	glog.V(4).Infof("Delete  class of %s on %s", cidr, ifb)
	if _, err := e.Command("tc", "class", "del", "dev", ifb, "parent", "1:", "classid", class).CombinedOutput(); err != nil {
		return err
	}
	return nil
}

// Get CIDRs from ifb's filters
func getCIDRs(ifb string) ([]string, error) {
	e := exec.New()
	data, err := e.Command("tc", "filter", "show", "dev", ifb).CombinedOutput()
	if err != nil {
		return nil, err
	}

	result := []string{}
	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}
		if strings.Contains(line, "match") {
			parts := strings.Split(line, " ")
			// expected tc line:
			// match <cidr> at <number>
			if len(parts) != 4 {
				return nil, fmt.Errorf("unexpected output: %v", parts)
			}
			cidr, err := asciiCIDR(parts[1])
			if err != nil {
				return nil, err
			}
			result = append(result, cidr)
		}
	}
	return result, nil
}

// Delete classes in the ifb which is not in the CIDR list
func DeleteExtraChaos(egressPodsCIDRs, ingressPodsCIDRs []string, firstIFB, secondIFB int) error {
	//delete extra chaos of egress
	first := fmt.Sprintf("ifb%d", firstIFB)
	second := fmt.Sprintf("ifb%d", secondIFB)

	egressCIDRsets := sliceToSets(egressPodsCIDRs)
	ifb0CIDRs, err := getCIDRs(first)
	if err != nil {
		return err
	}
	for _, ifb0CIDR := range ifb0CIDRs {
		if !egressCIDRsets.Has(ifb0CIDR) {
			if err := Reset(ifb0CIDR, first); err != nil {
				return err
			}
		}
	}
	//delete extra chaos of ingress
	ingressCIDRsets := sliceToSets(ingressPodsCIDRs)
	ifb1CIDRs, err := getCIDRs(second)
	if err != nil {
		return err
	}
	for _, ifb1CIDR := range ifb1CIDRs {
		if !ingressCIDRsets.Has(ifb1CIDR) {
			if err := Reset(ifb1CIDR, second); err != nil {
				return err
			}
		}
	}
	return nil
}
