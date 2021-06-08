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

// EnsureDefaultForGameServer ensure some default fields of GameServer
func EnsureDefaultForGameServer(gs *v1alpha1.GameServer) *v1alpha1.GameServer {
	gsCopy := gs.DeepCopy()
	if gsCopy.Spec.Scheduling == "" {
		gsCopy.Spec.Scheduling = v1alpha1.MostAllocated
	}
	if gsCopy.Spec.Template.Spec.ServiceAccountName == "" {
		gsCopy.Spec.Template.Spec.ServiceAccountName = defaultServiceAccountName
	}
	if len(gsCopy.Annotations[ExternalNetworkKey]) == 0 {
		return gsCopy
	}
	for _, state := range gsCopy.Spec.ReadinessGates {
		if state == LBReadyKey {
			return gsCopy
		}
	}
	gsCopy.Spec.ReadinessGates = append(gsCopy.Spec.ReadinessGates, LBReadyKey)
	return gsCopy
}

// EnsureDefaultsForGameServerSet ensure some default fields of GameServerSet
func EnsureDefaultsForGameServerSet(gsSet *v1alpha1.GameServerSet) *v1alpha1.GameServerSet {
	gsSetCopy := gsSet.DeepCopy()
	// setting selector
	if gsSetCopy.Spec.Selector == nil {
		gsSetCopy.Spec.Selector = &metav1.LabelSelector{}
	}
	if gsSetCopy.Spec.Selector.MatchLabels == nil {
		gsSetCopy.Spec.Selector.MatchLabels = map[string]string{
			carrierutil.GameServerSetLabelKey: gsSetCopy.Name,
		}
	}
	// setting scheduling strategy
	if gsSetCopy.Spec.Scheduling == "" {
		gsSetCopy.Spec.Scheduling = v1alpha1.MostAllocated
	}
	if gsSetCopy.Spec.Template.Spec.Template.Spec.ServiceAccountName == "" {
		gsSetCopy.Spec.Template.Spec.Template.Spec.ServiceAccountName = defaultServiceAccountName
	}
	return gsSetCopy
}

// EnsureDefaultsForSquad ensure some default fields of Squad
func EnsureDefaultsForSquad(squad *v1alpha1.Squad) *v1alpha1.Squad {
	squadCopy := squad.DeepCopy()
	// setting revision history limit
	if squadCopy.Spec.RevisionHistoryLimit == nil {
		squadCopy.Spec.RevisionHistoryLimit = new(int32)
		*squadCopy.Spec.RevisionHistoryLimit = 10
	}
	// setting selector
	if squadCopy.Spec.Selector == nil {
		squadCopy.Spec.Selector = &metav1.LabelSelector{}
	}
	if squadCopy.Spec.Selector.MatchLabels == nil {
		squadCopy.Spec.Selector.MatchLabels = map[string]string{
			carrierutil.SquadNameLabelKey: squadCopy.Name,
		}
	}
	// setting update strategy
	strategy := &squadCopy.Spec.Strategy
	// Set default v1alpha1.RollingUpdateSquadStrategyType as RollingUpdate.
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
	if squadCopy.Spec.Scheduling == "" {
		squadCopy.Spec.Scheduling = v1alpha1.MostAllocated
	}
	if squadCopy.Spec.Template.Spec.Template.Spec.ServiceAccountName == "" {
		squadCopy.Spec.Template.Spec.Template.Spec.ServiceAccountName = defaultServiceAccountName
	}
	return squadCopy
}
