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
	"fmt"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8testing "k8s.io/kubernetes/pkg/scheduler/testing"

	carrierv1alpha1 "github.com/ocgi/carrier/pkg/apis/carrier/v1alpha1"
	carrierutil "github.com/ocgi/carrier/pkg/util"
)

func TestEnsurePod(t *testing.T) {
	testCases := []struct {
		name   string
		pod    *v1.Pod
		newPod *v1.Pod
		opts   []option
	}{
		{
			name:   "side car already exists",
			pod:    addSidecar(defaultTestPod()).Obj(),
			newPod: addSidecar(defaultTestPod()).Obj(),
		},
		{
			name:   "not game server pod",
			pod:    k8testing.MakePod().Obj(),
			newPod: k8testing.MakePod().Obj(),
		},
		{
			name:   "game server pod, empty option",
			pod:    defaultTestPod().Obj(),
			newPod: emptyOption().Obj(),
		},
		{
			name:   "game server pod, health check",
			pod:    defaultTestPod().Obj(),
			newPod: healthOption().Obj(),
			opts: []option{
				WithHealthCheck(),
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pod := EnsurePod(tc.pod, nil, tc.opts...)
			if !reflect.DeepEqual(pod, tc.newPod) {
				t.Errorf("old: %v", tc.pod.Spec.Containers)
				t.Errorf("\ndesired:\n%v\nactual:\n%v", tc.newPod.Spec.Containers, pod.Spec.Containers)
			}
		})
	}
}

func TestEnsureSquad(t *testing.T) {
	actual := EnsureDefaultsForSquad(defaultSquad())
	desired := filledSquad()
	if !reflect.DeepEqual(actual, desired) {
		t.Errorf("\ndesired:\n%v\nactual:\n%v", desired, actual)
	}
}

// addSidecar add side car to test pod
func addSidecar(pw *k8testing.PodWrapper) *k8testing.PodWrapper {
	pw.Spec.Containers = append(pw.Spec.Containers, v1.Container{
		Name:            sdkServerSidecarName,
		ImagePullPolicy: v1.PullIfNotPresent,
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      fmt.Sprintf("%s-%s", defaultServiceAccountName, "token-2kx9e"),
				ReadOnly:  true,
				MountPath: mountPath,
			},
		},
	})
	return pw
}

// addHealthCheck add heal check.
func addHealthCheck(pw *k8testing.PodWrapper) *k8testing.PodWrapper {
	livenessProbe := &v1.Probe{
		Handler: v1.Handler{
			HTTPGet: &v1.HTTPGetAction{
				Path: "/healthz",
				Port: intstr.FromInt(8080),
			},
		},
		InitialDelaySeconds: 3,
		TimeoutSeconds:      1,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}
	for i, container := range pw.Pod.Spec.Containers {
		if container.Name != sdkServerSidecarName {
			continue
		}
		pw.Pod.Spec.Containers[i].LivenessProbe = livenessProbe
	}
	return pw
}

func defaultTestPod() *k8testing.PodWrapper {
	pw := k8testing.MakePod().Label(carrierutil.GameServerPodLabelKey, "test")
	pw.Spec.Containers = append(pw.Spec.Containers, v1.Container{
		Name: "test",
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      fmt.Sprintf("%s-%s", defaultServiceAccountName, "token-2kx9e"),
				ReadOnly:  false,
				MountPath: mountPath,
			},
		},
	})
	pw.Spec.ServiceAccountName = defaultServiceAccountName
	pw.Spec.Volumes = []v1.Volume{
		{
			Name:         fmt.Sprintf("%s-%s", defaultServiceAccountName, "token-2kx9e"),
			VolumeSource: v1.VolumeSource{},
		},
	}
	return pw
}

func emptyOption() *k8testing.PodWrapper {
	pw := defaultTestPod()
	return addSidecar(pw)
}

func healthOption() *k8testing.PodWrapper {
	pw := defaultTestPod()
	return addHealthCheck(addSidecar(pw))
}

func defaultSquad() *carrierv1alpha1.Squad {
	var port int32 = 1000
	return &carrierv1alpha1.Squad{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: carrierv1alpha1.SquadSpec{
			Template: carrierv1alpha1.GameServerTemplateSpec{
				Spec: carrierv1alpha1.GameServerSpec{
					Ports: []carrierv1alpha1.GameServerPort{
						{
							Name:          "test",
							ContainerPort: &port,
						},
					},
				},
			},
		},
	}
}

func filledSquad() *carrierv1alpha1.Squad {
	ratio := intstr.FromString("25%")
	var his int32 = 10
	var port int32 = 1000
	return &carrierv1alpha1.Squad{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: carrierv1alpha1.SquadSpec{
			Strategy: carrierv1alpha1.SquadStrategy{
				Type: carrierv1alpha1.RollingUpdateSquadStrategyType,
				RollingUpdate: &carrierv1alpha1.RollingUpdateSquad{
					MaxUnavailable: &ratio,
					MaxSurge:       &ratio,
				},
			},
			Scheduling: carrierv1alpha1.MostAllocated,
			Template: carrierv1alpha1.GameServerTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
				},
				Spec: carrierv1alpha1.GameServerSpec{
					Ports: []carrierv1alpha1.GameServerPort{
						{
							Name:          "test",
							ContainerPort: &port,
							PortPolicy:    carrierv1alpha1.LoadBalancer,
						},
					},
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							ServiceAccountName: defaultServiceAccountName,
						},
					},
				},
			},
			RevisionHistoryLimit: &his,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					carrierutil.SquadNameLabelKey: "test",
				},
			},
		},
	}
}
