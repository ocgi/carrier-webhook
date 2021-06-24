// Copyright 2021 The OCGI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package webhook

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/ocgi/carrier/pkg/apis/carrier/v1alpha1"
	carrierutil "github.com/ocgi/carrier/pkg/util"
)

// EnsurePod add side car to the pod and create patch.
func EnsurePod(pod *corev1.Pod, f func(*corev1.Pod), opts ...option) *corev1.Pod {
	if sideCarExist(pod) || !gameServerPod(pod) {
		return pod
	}
	podCopy := pod.DeepCopy()
	// add pod to containers
	if f != nil {
		f(podCopy)
	}
	sideCar := defaultSideCar(podCopy)
	for _, opt := range opts {
		opt(sideCar)
	}
	podCopy.Spec.Containers = append(podCopy.Spec.Containers, *sideCar)
	return podCopy
}

// EnsureDefaultForGameServer ensure some default fields of GameServer
func EnsureDefaultForGameServer(gs *v1alpha1.GameServer) *v1alpha1.GameServer {
	gsCopy := gs.DeepCopy()
	ensureLBReadinessGates(gsCopy)
	ensureDefaultSchedulingPolicy(&gsCopy.Spec.Scheduling)
	ensureDefaultServiceAccount(&gsCopy.Spec)
	ensureDefaultPortType(&gsCopy.Spec)
	return gsCopy
}

// EnsureDefaultsForGameServerSet ensure some default fields of GameServerSet
func EnsureDefaultsForGameServerSet(gsSet *v1alpha1.GameServerSet) *v1alpha1.GameServerSet {
	gsSetCopy := gsSet.DeepCopy()
	ensureDefaultTemplateLabel(&gsSetCopy.Spec.Template, carrierutil.GameServerSetLabelKey, gsSetCopy.Name)
	if gsSetCopy.Spec.Selector == nil {
		gsSetCopy.Spec.Selector = &metav1.LabelSelector{}
	}
	ensureDefaultSelector(gsSetCopy.Spec.Selector, carrierutil.GameServerSetLabelKey, gsSetCopy.Name)
	ensureDefaultSchedulingPolicy(&gsSetCopy.Spec.Scheduling)
	ensureDefaultServiceAccount(&gsSetCopy.Spec.Template.Spec)
	ensureDefaultPortType(&gsSetCopy.Spec.Template.Spec)
	return gsSetCopy
}

// EnsureDefaultsForSquad ensure some default fields of Squad
func EnsureDefaultsForSquad(squad *v1alpha1.Squad) *v1alpha1.Squad {
	squadCopy := squad.DeepCopy()
	ensureDefaultRevisionHistoryLimit(&squadCopy.Spec)
	ensureDefaultStrategy(&squadCopy.Spec.Strategy)
	if squadCopy.Spec.Selector == nil {
		squadCopy.Spec.Selector = &metav1.LabelSelector{}
	}
	ensureDefaultSelector(squadCopy.Spec.Selector, carrierutil.SquadNameLabelKey, squadCopy.Name)
	ensureDefaultSchedulingPolicy(&squadCopy.Spec.Scheduling)
	ensureDefaultServiceAccount(&squadCopy.Spec.Template.Spec)
	ensureDefaultPortType(&squadCopy.Spec.Template.Spec)
	return squadCopy
}

// sideCarExist checks if side car already exist
func sideCarExist(pod *corev1.Pod) bool {
	for _, container := range pod.Spec.Containers {
		if container.Name == sdkserverSidecarName {
			return true
		}
	}
	return false
}

func gameServerPod(pod *corev1.Pod) bool {
	if pod.Labels == nil {
		return false
	}
	return pod.Labels[carrierutil.GameServerPodLabelKey] != ""
}

// defaultSideCar return a simple sidecar caontainer
func defaultSideCar(pod *corev1.Pod) *corev1.Container {
	sideCar := corev1.Container{
		Name:            sdkserverSidecarName,
		ImagePullPolicy: corev1.PullIfNotPresent,
	}
	sideCar.VolumeMounts = buildSideCarVolumeMount(pod)
	return &sideCar
}

// buildSideCarVolumeMount build volume mount of service account.
func buildSideCarVolumeMount(pod *corev1.Pod) []corev1.VolumeMount {
	volumeName := ""
	for _, v := range pod.Spec.Volumes {
		if strings.Contains(v.Name, pod.Spec.ServiceAccountName+"-token") {
			volumeName = v.Name
			break
		}
	}
	if volumeName == "" {
		return nil
	}
	return []corev1.VolumeMount{
		{
			MountPath: mountPath,
			Name:      volumeName,
			ReadOnly:  true,
		},
	}
}

// ensureLBReadinessGates LB readinessGates if using LB
func ensureLBReadinessGates(gs *v1alpha1.GameServer) {
	if len(gs.Annotations) == 0 {
		return
	}
	if len(gs.Annotations[ExternalNetworkKey]) == 0 {
		return
	}
	for _, state := range gs.Spec.ReadinessGates {
		if state == LBReadyKey {
			return
		}
	}
	gs.Spec.ReadinessGates = append(gs.Spec.ReadinessGates, LBReadyKey)
}

// ensureDefaultPortType ensure default policyType of GameServer: LoaderBalancer
func ensureDefaultPortType(gsSpec *v1alpha1.GameServerSpec) {
	for i, port := range gsSpec.Ports {
		if len(port.PortPolicy) == 0 && port.HostPort == nil && port.HostPortRange == nil {
			gsSpec.Ports[i].PortPolicy = v1alpha1.LoadBalancer
		}
	}
}

// ensureDefaultServiceAccount ensure default serviceAccount name
func ensureDefaultServiceAccount(gsSpec *v1alpha1.GameServerSpec) {
	if gsSpec.Template.Spec.ServiceAccountName == "" {
		gsSpec.Template.Spec.ServiceAccountName = defaultServiceAccountName
	}
}

// ensureDefaultServiceAccount ensure default scheduling strategy
func ensureDefaultSchedulingPolicy(strategy *v1alpha1.SchedulingStrategy) {
	// setting scheduling strategy
	if *strategy == "" {
		*strategy = v1alpha1.MostAllocated
	}
}

// ensureDefaultSelector ensure default label selector
func ensureDefaultSelector(selector *metav1.LabelSelector, kind, name string) {
	// setting selector
	if selector.MatchLabels == nil {
		selector.MatchLabels = map[string]string{
			kind: name,
		}
	}
}

// ensureDefaultTemplateLabel ensure default label
func ensureDefaultTemplateLabel(gameServerTemplate *v1alpha1.GameServerTemplateSpec, kind, name string) {
	if len(gameServerTemplate.Labels) == 0 {
		gameServerTemplate.Labels = map[string]string{kind: name}
	}
}

// ensureDefaultStrategy ensure default update policy.
func ensureDefaultStrategy(strategy *v1alpha1.SquadStrategy) {
	if strategy.Type == "" {
		strategy.Type = v1alpha1.RollingUpdateSquadStrategyType
	}
	if strategy.Type == v1alpha1.RollingUpdateSquadStrategyType {
		if strategy.RollingUpdate == nil {
			rollingUpdate := v1alpha1.RollingUpdateSquad{}
			strategy.RollingUpdate = &rollingUpdate
		}
		if strategy.RollingUpdate.MaxUnavailable == nil {
			// Set default MaxUnavailable as 25% by default.
			maxUnavailable := intstr.FromString("25%")
			strategy.RollingUpdate.MaxUnavailable = &maxUnavailable
		}
		if strategy.RollingUpdate.MaxSurge == nil {
			// Set default MaxSurge as 25% by default.
			maxSurge := intstr.FromString("25%")
			strategy.RollingUpdate.MaxSurge = &maxSurge
		}
	}
}

// ensureDefaultRevisionHistoryLimit revisionHistoryLimit to 10.
func ensureDefaultRevisionHistoryLimit(squadSpec *v1alpha1.SquadSpec) {
	if squadSpec.RevisionHistoryLimit == nil {
		squadSpec.RevisionHistoryLimit = new(int32)
	}
	*squadSpec.RevisionHistoryLimit = 10
}

// CopyDefaultsForSquad copy some default fields of Squad
func CopyDefaultsForSquad(oldSquad, newSquad *v1alpha1.Squad) *v1alpha1.Squad {
	squadCopy := newSquad.DeepCopy()
	if squadCopy.Spec.Template.Spec.Template.Spec.ServiceAccountName == "" {
		squadCopy.Spec.Template.Spec.Template.Spec.ServiceAccountName = oldSquad.Spec.Template.Spec.Template.Spec.ServiceAccountName
	}
	ensureDefaultPortType(&squadCopy.Spec.Template.Spec)
	return squadCopy
}
