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

type Shaper interface {
	// Reconcile the interface managed by this shaper with the state on the ground.
	ReconcileIngressInterface(ingressChaosInfo ChaosInfo) error
	// Clear the ingress interface
	ClearIngressInterface() error
	// Reconcile a CIDR managed by this shaper with the state on the ground
	ReconcileIngressCIDR(cidr string, ingressChaosInfo ChaosInfo) error
	// Reconcile the mirroring from the interface to ifb
	ReconcileIngressMirroring(cidr string) error
	// Reconcile the interface managed by this shaper with the state on the ground.
	ReconcileEgressInterface(egressChaosInfo ChaosInfo) error
	// Clear the egress interface
	ClearEgressInterface() error
	// Reconcile a CIDR managed by this shaper with the state on the ground
	ReconcileEgressCIDR(cidr string, egressChaosInfo ChaosInfo) error
	// Reconcile the mirroring from the interface to ifb
	ReconcileEgressMirroring(cidr string) error

	ExecTcChaos(isIngress bool, info ChaosInfo) error
}
