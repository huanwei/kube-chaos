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

type Shaper interface {
	// Reconcile the interface managed by this shaper with the state on the ground.
	ReconcileInterface(egressChaosInfo, ingressChaosInfo ChaosInfo) error
	// Reconcile a CIDR managed by this shaper with the state on the ground
	ReconcileCIDR(cidr string, egressChaosInfo, ingressChaosInfo ChaosInfo) error

	Loss(percentage, relate string) error

	Delay(time, deviation string) error

	Duplicate(percentage string) error

	Reorder(time, percentage, relate string) error

	Corrupt(percentage string) error

	ExecTcChaos(info ChaosInfo) error
}
