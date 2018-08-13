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
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	"github.com/huanwei/kube-chaos/pkg/exec"
	"github.com/huanwei/kube-chaos/pkg/sets"

	"errors"
	"github.com/golang/glog"
)

// tcShaper provides an implementation of the Shaper interface on Linux using the 'tc' tool.
// Uses the hierarchical token bucket queuing discipline (htb), this requires Linux 2.4.20 or newer
// or a custom kernel with that queuing discipline backported.
type tcShaper struct {
	e              exec.Interface
	iface          string
	ingressClassid string
	egressClassid  string
}

func NewTCShaper(iface string) Shaper {
	shaper := &tcShaper{
		e:     exec.New(),
		iface: iface,
	}
	return shaper
}

func (t *tcShaper) execAndLog(cmdStr string, args ...string) error {
	glog.V(4).Infof("Running: %s %s", cmdStr, strings.Join(args, " "))
	cmd := t.e.Command(cmdStr, args...)
	out, err := cmd.CombinedOutput()
	glog.V(4).Infof("Output from tc: %s", string(out))
	return err
}

func (t *tcShaper) nextClassID(ifb string) (int, error) {
	data, err := t.e.Command("tc", "class", "show", "dev", ifb).CombinedOutput()
	if err != nil {
		return -1, err
	}

	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	classes := sets.String{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// skip empty lines
		if len(line) == 0 {
			continue
		}
		parts := strings.Split(line, " ")
		// todo - fix
		// expected tc line:
		// class htb 1:1 root prio 0 rate 1000Kbit ceil 1000Kbit burst 1600b cburst 1600b
		// class htb 1:1 root leaf 2: prio 0 rate 800000Kbit ceil 800000Kbit burst 1600b cburst 1600b
		if len(parts) != 14 && len(parts) != 16 {
			return -1, fmt.Errorf("unexpected output from tc: %s (%v)", scanner.Text(), parts)
		}
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

func findCIDRClass(cidr, ifb string) (class, handle string, found bool, err error) {
	e := exec.New()
	data, err := e.Command("tc", "filter", "show", "dev", ifb).CombinedOutput()
	if err != nil {
		return "", "", false, err
	}

	hex, err := hexCIDR(cidr)
	if err != nil {
		return "", "", false, err
	}
	spec := fmt.Sprintf("match %s", hex)

	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	filter := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}
		if strings.HasPrefix(line, "filter") {
			filter = line
			continue
		}
		if strings.Contains(line, spec) {
			parts := strings.Split(filter, " ")
			//todo - fix
			// expected tc line:
			// filter parent 1: protocol ip pref 1 u32 fh 800::800 order 2048 key ht 800 bkt 0 flowid 1:1
			if len(parts) != 19 {
				return "", "", false, fmt.Errorf("unexpected output from tc: %s %d (%v)", filter, len(parts), parts)
			}
			return parts[18], parts[9], true, nil
		}
	}
	return "", "", false, nil
}

func (t *tcShaper) classExists(classid, ifb string) (bool, error) {
	data, err := t.e.Command("tc", "class", "show", "dev", ifb).CombinedOutput()
	if err != nil {
		return false, err
	}
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

func (t *tcShaper) ReconcileIngressCIDR(cidr string, ingressChaosInfo ChaosInfo) error {
	glog.V(4).Infof("Shaper CIDR %s with ingressChaosInfo %s", cidr, ingressChaosInfo)
	return nil
}

func (t *tcShaper) ReconcileEgressCIDR(cidr string, egressChaosInfo ChaosInfo) error {
	glog.V(4).Infof("Shaper CIDR %s with egressChaosInfo %s", cidr, egressChaosInfo)
	return nil
}

func (t *tcShaper) ReconcileIngressInterface(ingressChaosInfo ChaosInfo) error {
	e := exec.New()

	// For ingress test
	data, err := e.Command("tc", "qdisc", "add", "dev", "ifb1", "parent",
		t.ingressClassid, "netem").CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s\n%s", err, data)
		return err
	} else {
		glog.Infof("Ingress netem added")
	}
	return nil
}

func (t *tcShaper) ReconcileEgressInterface(egressChaosInfo ChaosInfo) error {
	e := exec.New()

	// For egress test
	data, err := e.Command("tc", "qdisc", "add", "dev", "ifb0", "parent",
		t.egressClassid, "netem").CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s\n%s", err, data)
		return err
	} else {
		glog.Infof("Egress netem added")
	}
	return nil
}

func (t *tcShaper) ClearIngressInterface() error {
	e := exec.New()

	glog.Infof("Clear ingress interface of class id: " + t.ingressClassid)
	e.Command("tc", "qdisc", "del", "dev", "ifb1", "parent",
		t.ingressClassid).CombinedOutput()

	return nil
}

func (t *tcShaper) ClearEgressInterface() error {
	e := exec.New()

	glog.Infof("Clear egress interface of class id: " + t.egressClassid)
	e.Command("tc", "qdisc", "del", "dev", "ifb0", "parent",
		t.egressClassid).CombinedOutput()

	return nil
}

func (t *tcShaper) ReconcileIngressMirroring(cidr string) error {
	e := exec.New()

	// Tested highest settable rate on tc
	rate := "4gbps"
	// Tested queue size
	size := "1600"

	class, _, isFind, err := findCIDRClass(cidr, "ifb1")
	if err != nil {
		glog.Errorf("Error when finding class id: %s", err)
		return err
	}

	isExist := false
	if isFind {
		isExist, err = t.classExists(class, "ifb1")
		if err != nil {
			glog.Errorf("Error when checking class id existence: %s", err)
			return err
		}
		if !isExist {
			// Class not exist but filter was added, delete the useless filter
			// tc filter del dev ifb1 parent 1:
			glog.Infof("Deleting useless filter at ifb1")
			data, err := e.Command("tc", "filter", "del", "dev", "ifb1", "parent",
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
		glog.Infof("IFB1 has already been initialized")
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

		// Mirror the egress of caliXXX to ifb1
		data, err = e.Command("tc", "filter", "add", "dev", t.iface, "parent", "1:", "protocol", "ip",
			"prio", "1", "u32", "match", "u32", "0", "0", "flowid", "1:1",
			"action", "mirred", "egress", "redirect", "dev", "ifb1").CombinedOutput()
		if err != nil {
			glog.Errorf("TC exec error: %s\n%s", err, data)
			return err
		} else {
			glog.Infof("Egress of %s mirrored to ifb1", t.iface)
		}

		// Get an unused classid
		classid, err := t.nextClassID("ifb1")
		if err != nil {
			return err
		} else {
			t.ingressClassid = fmt.Sprintf("1:%d", classid)
			glog.Infof("IFB1 get class %s", t.ingressClassid)
		}

		// Add a filter
		data, err = e.Command("tc", "filter", "add", "dev", "ifb1", "parent", "1:0", "protocol", "ip",
			"prio", "1", "u32", "match", "ip", "dst", cidr, "flowid", t.ingressClassid,
		).CombinedOutput()
		if err != nil {
			glog.Errorf("TC exec error: %s\n%s", err, data)
			return err
		} else {
			glog.Infof("Filter added")
		}

		// Create a class at ifb1
		err = t.makeNewClass(rate, "ifb1", classid)
		if err != nil {
			glog.Errorf("TC exec error: %s\n", err)
			return err
		} else {
			glog.Infof("IFB1 class added")
		}
	}

	return nil
}

func (t *tcShaper) ReconcileEgressMirroring(cidr string) error {
	e := exec.New()

	// Tested highest settable rate on tc
	rate := "4gbps"

	class, _, isFind, err := findCIDRClass(cidr, "ifb0")
	if err != nil {
		glog.Errorf("Error when finding class id: %s", err)
		return err
	}

	isExist := false
	if isFind {
		isExist, err = t.classExists(class, "ifb0")
		if err != nil {
			glog.Errorf("Error when checking class id existence: %s", err)
			return err
		}
		if !isExist {
			// Class not exist but filter was added, delete the useless filter
			// tc filter del dev ifb0 parent 1:
			glog.Infof("Deleting useless filter at ifb0")
			data, err := e.Command("tc", "filter", "del", "dev", "ifb0", "parent",
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
		glog.Infof("IFB0 has already been initialized")
		t.egressClassid = class
	} else {

		// Check if ingress was already added.
		data, err := e.Command("tc", "qdisc", "show", "dev", t.iface, "ingress").CombinedOutput()
		if err != nil {
			glog.Errorf("TC exec error: %s\n%s", err, data)
			return err
		}
		scanner := bufio.NewScanner(bytes.NewBuffer(data))
		ingressAdded := false
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			// skip empty lines
			if len(line) == 0 {
				continue
			}
			parts := strings.Split(line, " ")
			// expected tc line:
			// qdisc noqueue 0: root refcnt 2
			// qdisc ingress ffff: parent ffff:fff1 ----------------
			if parts[1] == "ingress" {
				ingressAdded = true
				glog.Infof("Ingress was already added")
				break
			}
		}

		// Add qdisc of ingress
		if !ingressAdded {
			data, err = e.Command("tc", "qdisc", "add", "dev", t.iface, "ingress").CombinedOutput()
			if err != nil {
				glog.Errorf("TC exec error: %s\n%s", err, data)
				return err
			} else {
				glog.Infof("Ingress added")
			}
		}

		// Mirror the ingress of caliXXX to ifb0
		data, err = e.Command("tc", "filter", "add", "dev", t.iface, "parent", "ffff:", "protocol", "ip",
			"prio", "1", "u32", "match", "u32", "0", "0", "flowid", "1:1",
			"action", "mirred", "egress", "redirect", "dev", "ifb0").CombinedOutput()
		if err != nil {
			glog.Errorf("TC exec error: %s\n%s", err, data)
			return err
		} else {
			glog.Infof("Ingress of %s mirrored to ifb0", t.iface)
		}

		// Get an unused classid
		classid, err := t.nextClassID("ifb0")
		if err != nil {
			return err
		} else {
			t.egressClassid = fmt.Sprintf("1:%d", classid)
			glog.Infof("IFB0 get class %s", t.egressClassid)
		}

		// Add a filter
		data, err = e.Command("tc", "filter", "add", "dev", "ifb0", "parent", "1:0", "protocol", "ip",
			"prio", "1", "u32", "match", "ip", "src", cidr, "flowid", t.egressClassid,
		).CombinedOutput()
		if err != nil {
			glog.Errorf("TC exec error: %s\n%s", err, data)
			return err
		} else {
			glog.Infof("Filter added")
		}

		// Create a class
		err = t.makeNewClass(rate, "ifb0", classid)
		if err != nil {
			glog.Errorf("TC exec error: %s\n", err)
			return err
		} else {
			glog.Infof("IFB0 class added")
		}
	}
	return nil
}

func (t *tcShaper) Rate(classid, ifb string, rate string) error {
	// For test
	glog.Infof("Adding rate %s to interface: %s", rate, ifb)
	t.changeClass(rate, ifb, classid)

	return nil
}

func (t *tcShaper) Loss(classid, ifb string, percentage, relate string) error {
	// tc  qdisc  add  dev  eth0  root  netem  loss  1%  30%
	e := exec.New()

	// For test
	glog.Infof("Adding loss %s,%s to interface: %s", percentage, relate, ifb)
	data, err := e.Command("tc", "qdisc", "change", "dev", ifb, "parent",
		classid, "netem", "loss", percentage, relate).CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s\n%s", err, data)
		return err
	} else {
		glog.Infof("Loss added")
	}

	return nil
}

func (t *tcShaper) Delay(classid, ifb string, time, deviation string) error {
	// tc  qdisc  add  dev  eth0  root  netem  delay  100ms  10ms  30%
	//												 basis	devi  devirate
	e := exec.New()

	// For test
	glog.Infof("Adding delay %s, %s to interface: %s", time, deviation, ifb)
	data, err := e.Command("tc", "qdisc", "change", "dev", ifb, "parent",
		classid, "netem", "delay", time, deviation).CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s\n%s", err, data)
		return err
	} else {
		glog.Infof("Delay added")
	}

	return nil
}

func (t *tcShaper) Duplicate(classid, ifb string, percentage string) error {
	// tc  qdisc  add  dev  eth0  root  netem  duplicate 1%
	e := exec.New()

	// For test
	glog.Infof("Adding duplicate %s to interface: %s", percentage, ifb)
	data, err := e.Command("tc", "qdisc", "change", "dev", ifb, "parent",
		classid, "netem", "duplicate", percentage).CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s ,\n%s", err, data)
		return err
	} else {
		glog.Infof("Duplicate added")
	}

	return nil
}

func (t *tcShaper) Reorder(classid, ifb string, time, percentage, relate string) error {
	// tc  qdisc  change  dev  eth0  root  netem  delay  10ms   reorder  25%  50%
	e := exec.New()

	// For test
	glog.Infof("Adding reorder %s, percent %s, relate %s to interface: %s", time, percentage, relate, ifb)
	data, err := e.Command("tc", "qdisc", "change", "dev", ifb, "parent",
		classid, "netem", "delay", time, "reorder", percentage, relate).CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s ,\n%s", err, data)
		return err
	} else {
		glog.Infof("Reorder added")
	}

	return nil
}

func (t *tcShaper) Corrupt(classid, ifb string, percentage string) error {
	// tc  qdisc  add  dev  eth0  root  netem  corrupt  0.2%
	e := exec.New()

	// For test
	glog.Infof("Adding corrupt %s to interface: %s", percentage, ifb)
	data, err := e.Command("tc", "qdisc", "change", "dev", ifb, "parent",
		classid, "netem", "corrupt", percentage).CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s ,\n%s", err, data)
		return err
	} else {
		glog.Infof("Corrupt added")
	}

	return nil
}

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

func (t *tcShaper) ExecTcChaos(isIngress bool, info ChaosInfo) error {
	var classid, ifb string
	if isIngress {
		classid = t.ingressClassid
		ifb = "ifb1"
	} else {
		classid = t.egressClassid
		ifb = "ifb0"
	}
	t.Rate(classid, ifb, info.Rate)
	if info.Delay.Set == "yes" {
		return t.Delay(classid, ifb, info.Delay.Time, info.Delay.Variation)
	}
	if info.Loss.Set == "yes" {
		return t.Loss(classid, ifb, info.Loss.Percentage, info.Loss.Relate)
	}
	if info.Duplicate.Set == "yes" {
		return t.Duplicate(classid, ifb, info.Duplicate.Percentage)
	}
	if info.Reorder.Set == "yes" {
		return t.Reorder(classid, ifb, info.Reorder.Time, info.Reorder.Percengtage, info.Reorder.Relate)
	}
	if info.Corrupt.Set == "yes" {
		return t.Corrupt(classid, ifb, info.Corrupt.Percentage)
	}
	return errors.New("No Chaos Info set")
}

// Remove a bandwidth limit for a particular CIDR on a particular network interface
func reset(cidr, ifb string) error {
	e := exec.New()
	class, handle, found, err := findCIDRClass(cidr, ifb)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("Failed to find cidr: %s on interface: %s", cidr, ifb)
	}
	glog.V(4).Infof("Delete  filter of %s on ifb0", cidr)
	if _, err := e.Command("tc", "filter", "del",
		"dev", ifb,
		"parent", "1:",
		"proto", "ip",
		"prio", "1",
		"handle", handle, "u32").CombinedOutput(); err != nil {
		return err
	}
	glog.V(4).Infof("Delete  class of %s on ifb0", cidr)
	if _, err := e.Command("tc", "class", "del", "dev", ifb, "parent", "1:", "classid", class).CombinedOutput(); err != nil {
		return err
	}
	return nil
}

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

func DeleteExtraChaos(egressPodsCIDRs, ingressPodsCIDRs []string) error {
	//delete extra chaos of egress
	egressCIDRsets := sliceToSets(egressPodsCIDRs)
	ifb0CIDRs, err := getCIDRs("ifb0")
	if err != nil {
		return err
	}
	for _, ifb0CIDR := range ifb0CIDRs {
		if !egressCIDRsets.Has(ifb0CIDR) {
			if err := reset(ifb0CIDR, "ifb0"); err != nil {
				return err
			}
		}
	}
	//delete extra chaos of ingress
	ingressCIDRsets := sliceToSets(ingressPodsCIDRs)
	ifb1CIDRs, err := getCIDRs("ifb1")
	if err != nil {
		return err
	}
	for _, ifb1CIDR := range ifb1CIDRs {
		if !ingressCIDRsets.Has(ifb1CIDR) {
			if err := reset(ifb1CIDR, "ifb1"); err != nil {
				return err
			}
		}
	}
	return nil
}

func sliceToSets(slice []string) sets.String {
	ss := sets.String{}
	for _, s := range slice {
		ss.Insert(s)
	}
	return ss
}
