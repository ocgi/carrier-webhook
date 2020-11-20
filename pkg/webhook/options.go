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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// option defines func for sidecar to inject
type option func(*corev1.Container)

// WithResource add resource to sidecar
func WithResource(sc *SideCarConfig) option {
	return func(container *corev1.Container) {
		resource := map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    sc.CPU,
			corev1.ResourceMemory: sc.Memory,
		}
		container.Resources = corev1.ResourceRequirements{
			Limits:   resource,
			Requests: resource,
		}
	}
}

// WithImageName add image
func WithImageName(sc *SideCarConfig) option {
	return func(container *corev1.Container) {
		container.Image = sc.Image
	}
}

// WithHealthCheck inject health check
func WithHealthCheck() option {
	return func(container *corev1.Container) {
		livenessProbe := &corev1.Probe{
			Handler: corev1.Handler{
				HTTPGet: &corev1.HTTPGetAction{
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
		container.LivenessProbe = livenessProbe
	}
}

// WithEnvs add port envs to side car
func WithEnvs(pod *corev1.Pod) option {
	return func(container *corev1.Container) {
		container.Env = []corev1.EnvVar{
			{
				Name:  gsEnvKey,
				Value: pod.Name,
			},
			{
				Name: nsKey,
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
		}
	}
}

// WithArgs add port to side car args
func WithArgs(httpPort, grpcPort int) option {
	return func(container *corev1.Container) {
		container.Args = []string{
			fmt.Sprintf("--grpc-port=%v", grpcPort),
			fmt.Sprintf("--http-port=%v", httpPort),
			"--v=5",
		}
	}
}
