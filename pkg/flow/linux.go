//// +build linux

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
	e     exec.Interface
	iface string
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
		if len(parts) != 14 {
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

func (t *tcShaper) makeNewClass(rate, ifb string) (int, error) {
	class, err := t.nextClassID(ifb)
	if err != nil {
		return -1, err
	}
	if err := t.execAndLog("tc", "class", "add",
		"dev", ifb,
		"parent", "1:",
		"classid", fmt.Sprintf("1:%d", class),
		"htb", "rate", rate); err != nil {
		return -1, err
	}
	return class, nil
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

func (t *tcShaper) ReconcileCIDR(cidr, egressChaosInfo, ingressChaosInfo string) error {
	glog.V(4).Infof("Shaper CIDR %s with egressChaosInfo %s, ingressChaosInfo %s", cidr, egressChaosInfo, ingressChaosInfo)
	return nil
}

func (t *tcShaper) ReconcileInterface(egressChaosInfo, ingressChaosInfo string) error {
	e := exec.New()
	e.Command("tc", "qdisc", "del", "dev", t.iface, "root").CombinedOutput()

	glog.Infof("Adding netem to interface: %s", t.iface)
	// For test
	data, err := e.Command("tc", "qdisc", "add", "dev", t.iface, "root", "netem").CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s\n%s", err, data)
		return err
	} else {
		glog.Infof("Netem added")
	}
	return nil
}

func (t *tcShaper) Loss(percentage, relate string) error {
	// tc  qdisc  add  dev  eth0  root  netem  loss  1%  30%
	e := exec.New()
	glog.Infof("Adding loss %s,%s to interface: %s", percentage, relate, t.iface)
	// For test
	data, err := e.Command("tc", "qdisc", "change", "dev", t.iface, "root", "netem", "loss", percentage, relate).CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s\n%s", err, data)
		return err
	} else {
		glog.Infof("Loss added")
	}
	return nil
}

func (t *tcShaper) Delay(time, deviation string) error {
	// tc  qdisc  add  dev  eth0  root  netem  delay  100ms  10ms  30%
	//												 basis	devi  devirate
	e := exec.New()
	glog.Infof("Adding delay %s, %s to interface: %s", time, deviation, t.iface)
	// For test
	data, err := e.Command("tc", "qdisc", "change", "dev", t.iface, "root", "netem", "delay", time, deviation).CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s\n%s", err, data)
		return err
	} else {
		glog.Infof("Delay added")
	}
	return nil
}

func (t *tcShaper) Duplicate(percentage string) error {
	// tc  qdisc  add  dev  eth0  root  netem  duplicate 1%
	e := exec.New()
	glog.Infof("Adding duplicate %s to interface: %s", percentage, t.iface)
	// For test
	data, err := e.Command("tc", "qdisc", "change", "dev", t.iface, "root", "netem", "duplicate", percentage).CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s ,\n%s", err, data)
		return err
	} else {
		glog.Infof("Duplicate added")
	}
	return nil
}

func (t *tcShaper) Reorder(time, percentage, relate string) error {
	// tc  qdisc  change  dev  eth0  root  netem  delay  10ms   reorder  25%  50%
	e := exec.New()
	glog.Infof("Adding reorder %s, percent %s, relate %s to interface: %s", time, percentage, relate, t.iface)
	// For test
	data, err := e.Command("tc", "qdisc", "change", "dev", t.iface, "root", "netem", "delay", time, "reorder", percentage, relate).CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s ,\n%s", err, data)
		return err
	} else {
		glog.Infof("Duplicate added")
	}
	return nil
}

func (t *tcShaper) Corrupt(percentage string) error {
	// tc  qdisc  add  dev  eth0  root  netem  corrupt  0.2%
	e := exec.New()
	glog.Infof("Adding corrupt %s to interface: %s", percentage, t.iface)
	// For test
	data, err := e.Command("tc", "qdisc", "change", "dev", t.iface, "root", "netem", "corrupt", percentage).CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s ,\n%s", err, data)
		return err
	} else {
		glog.Infof("Corrupt added")
	}
	return nil
}

func (t *tcShaper) Clear(percentage, relate string) error {
	e := exec.New()
	glog.Infof("Deleting netem in interface: %s", t.iface)
	// For test
	data, err := e.Command("tc", "qdisc", "del", "dev", t.iface, "root", "netem").CombinedOutput()
	if err != nil {
		glog.Errorf("TC exec error: %s\n%s", err, data)
		return err
	} else {
		glog.Infof("Netem deleted")
	}
	return nil
}

func (t *tcShaper) ExecTcChaos(info TCChaosInfo) error {
	if info.Delay.Set == "yes" {
		return t.Delay(info.Delay.Time, info.Delay.Deviation)
	}
	if info.Loss.Set == "yes" {
		return t.Loss(info.Loss.Percentage, info.Loss.Relate)
	}
	if info.Duplicate.Set == "yes" {
		return t.Duplicate(info.Duplicate.Percentage)
	}
	if info.Reorder.Set == "yes" {
		return t.Reorder(info.Reorder.Time, info.Reorder.Percengtage, info.Reorder.Relate)
	}
	if info.Corrupt.Set == "yes" {
		return t.Corrupt(info.Corrupt.Percentage)
	}
	return errors.New("No tc Chaos Info set")
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

func (t *tcShaper) deleteInterface(class, ifb string) error {
	return t.execAndLog("tc", "qdisc", "delete", "dev", ifb, "root", "handle", class)
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
