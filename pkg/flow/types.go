package flow

import "github.com/huanwei/kube-chaos/pkg/exec"

// tcShaper provides an implementation of the Shaper interface on Linux using the 'tc' tool.
// Uses the hierarchical token bucket queuing discipline (htb), this requires Linux 2.4.20 or newer
// or a custom kernel with that queuing discipline backported.
type tcShaper struct {
	e              exec.Interface
	iface          string
	FirstIFB       string
	SecondIFB      string
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
