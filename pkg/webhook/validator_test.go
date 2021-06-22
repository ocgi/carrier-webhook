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
	"testing"

	corev1 "k8s.io/api/core/v1"

	carrierv1alpha1 "github.com/ocgi/carrier/pkg/apis/carrier/v1alpha1"
)

func Test_ValidateGameServer(t *testing.T) {
	var tcpport int32 = 10000
	for _, c := range []struct {
		name  string
		newGS *carrierv1alpha1.GameServer
		ok    bool
	}{
		{
			name: "with containerPort, success",
			newGS: defaultGS().SetPorts([]carrierv1alpha1.GameServerPort{
				{
					Name:          "dsport-tcp",
					ContainerPort: &tcpport,
					Protocol:      corev1.ProtocolTCP,
				},
			}).Obj(),
			ok: true,
		},
		{
			name: "with containerPortRange, success",
			newGS: defaultGS().SetPorts([]carrierv1alpha1.GameServerPort{
				{
					Name: "dsport-udp",
					ContainerPortRange: &carrierv1alpha1.PortRange{
						MinPort: 10000,
						MaxPort: 10099,
					},
					Protocol: corev1.ProtocolUDP,
				},
			}).Obj(),
			ok: true,
		},
		{
			name: "with both containerPortRange and containerPort, fail",
			newGS: defaultGS().SetPorts([]carrierv1alpha1.GameServerPort{
				{
					Name: "dsport-udp",
					ContainerPortRange: &carrierv1alpha1.PortRange{
						MinPort: 10000,
						MaxPort: 10099,
					},
					ContainerPort: &tcpport,
					Protocol:      corev1.ProtocolUDP,
				},
			}).Obj(),
			ok: false,
		},
		{
			name:  "without ports length 0, success",
			newGS: defaultGS().SetPorts([]carrierv1alpha1.GameServerPort{}).Obj(),
			ok:    true,
		},
		{
			name:  "with ports nil, success",
			newGS: defaultGS().SetPorts(nil).Obj(),
			ok:    true,
		},
		{
			name:  "container named server not exist",
			newGS: defaultGSWithoutContainerName().SetPorts(nil).Obj(),
			ok:    false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			errs := ValidateGameServer(c.newGS)
			if errs.ToAggregate() == nil != c.ok {
				t.Errorf("desired %v, get %v, ca", c.ok, errs.ToAggregate())
				return
			}
			t.Log(errs.ToAggregate())
		})
	}
}

func Test_ValidateGameServerUpdate(t *testing.T) {
	old := defaultGS()
	var tcpport int32 = 10000
	for _, c := range []struct {
		name  string
		newGS *carrierv1alpha1.GameServer
		ok    bool
	}{
		{
			name:  "image change, success",
			newGS: defaultGS().SetImage("test:latest1").Obj(),
			ok:    true,
		},
		{
			name:  "annotation change, success",
			newGS: defaultGS().SetAnnotations(map[string]string{"test": "latest1"}).Obj(),
			ok:    true,
		},
		{
			name:  "container name change, fail",
			newGS: defaultGS().SetContainerName("test1").Obj(),
			ok:    false,
		},
		{
			name: "ports change, fail",
			newGS: defaultGS().SetPorts([]carrierv1alpha1.GameServerPort{
				{
					Name: "dsport-udp",
					ContainerPortRange: &carrierv1alpha1.PortRange{
						MinPort: 10000,
						// changed maxPort
						MaxPort: 10199,
					},
					Protocol: corev1.ProtocolUDP,
				},
				{
					Name:          "dsport-tcp",
					ContainerPort: &tcpport,
					Protocol:      corev1.ProtocolTCP,
				},
			}).Obj(),
			ok: false,
		},
		{
			name: "ports not change, success",
			newGS: defaultGS().SetPorts([]carrierv1alpha1.GameServerPort{
				{
					Name: "dsport-udp",
					ContainerPortRange: &carrierv1alpha1.PortRange{
						MinPort: 10000,
						MaxPort: 10099,
					},
					Protocol: corev1.ProtocolUDP,
				},
				{
					Name:          "dsport-tcp",
					ContainerPort: &tcpport,
					Protocol:      corev1.ProtocolTCP,
				},
			}).Obj(),
			ok: true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			errs := ValidateGameServerUpdate(old.Obj(), c.newGS)
			if errs.ToAggregate() == nil != c.ok {
				t.Errorf("desired %v, get %v, ca", c.ok, errs.ToAggregate())
				return
			}
			t.Log(errs.ToAggregate())
		})
	}
}

func Test_ValidateGameServerSetUpdate(t *testing.T) {
	old := defaultGSS()
	for _, c := range []struct {
		name   string
		newGSS *carrierv1alpha1.GameServerSet
		ok     bool
	}{
		{
			name:   "change replicas, ok",
			newGSS: defaultGSS().SetReplicas(10).Obj(),
			ok:     true,
		},
		{
			name:   "change annotations, ok",
			newGSS: defaultGSS().SetAnnotations(map[string]string{"a": "a"}).Obj(),
			ok:     true,
		},
		{
			name:   "change image version, ok",
			newGSS: defaultGSS().SetImage("test:v2").Obj(),
			ok:     true,
		},
		{
			name:   "change image policy , ok",
			newGSS: defaultGSS().SetImagePolicy(corev1.PullAlways).Obj(),
			ok:     true,
		},
		{
			name:   "change others, !ok",
			newGSS: defaultGSS().SetContainerName("test").Obj(),
			ok:     false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			errs := ValidateGameServerSetUpdate(old.Obj(), c.newGSS)
			if errs.ToAggregate() == nil != c.ok {
				t.Errorf("desired %v, get %v, ca", c.ok, errs.ToAggregate())
				return
			}
			t.Log(errs.ToAggregate())
		})
	}
}

type GameServerWrapper struct {
	*carrierv1alpha1.GameServer
}

func (w *GameServerWrapper) SetImage(image string) *GameServerWrapper {
	w.Spec.Template.Spec.Containers[0].Image = image
	return w
}

func (w *GameServerWrapper) SetContainerName(name string) *GameServerWrapper {
	w.Spec.Template.Spec.Containers[0].Name = name
	return w
}

func (w *GameServerWrapper) SetAnnotations(ann map[string]string) *GameServerWrapper {
	w.Spec.Template.Annotations = ann
	return w
}

func (w *GameServerWrapper) SetPorts(ports []carrierv1alpha1.GameServerPort) *GameServerWrapper {
	w.Spec.Ports = ports
	return w
}

func (w *GameServerWrapper) Obj() *carrierv1alpha1.GameServer {
	return w.GameServer
}

type GameServerSetWrapper struct {
	*carrierv1alpha1.GameServerSet
}

func (w *GameServerSetWrapper) SetImage(image string) *GameServerSetWrapper {
	w.Spec.Template.Spec.Template.Spec.Containers[0].Image = image
	return w
}

func (w *GameServerSetWrapper) SetReplicas(replicas int32) *GameServerSetWrapper {
	w.Spec.Replicas = replicas
	return w
}

func (w *GameServerSetWrapper) SetContainerName(name string) *GameServerSetWrapper {
	w.Spec.Template.Spec.Template.Spec.Containers[0].Name = name
	return w
}

func (w *GameServerSetWrapper) SetImagePolicy(policy corev1.PullPolicy) *GameServerSetWrapper {
	w.Spec.Template.Spec.Template.Spec.Containers[0].ImagePullPolicy = policy
	return w
}

func (w *GameServerSetWrapper) SetAnnotations(ann map[string]string) *GameServerSetWrapper {
	w.Spec.Template.Annotations = ann
	return w
}

func (w *GameServerSetWrapper) Obj() *carrierv1alpha1.GameServerSet {
	return w.GameServerSet
}

func defaultGS() *GameServerWrapper {
	var tcpport int32 = 10000
	gs := &carrierv1alpha1.GameServer{
		Spec: carrierv1alpha1.GameServerSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "server",
							Image: "test:latest",
						},
					},
				},
			},
			Ports: []carrierv1alpha1.GameServerPort{
				{
					Name: "dsport-udp",
					ContainerPortRange: &carrierv1alpha1.PortRange{
						MinPort: 10000,
						MaxPort: 10099,
					},
					Protocol: corev1.ProtocolUDP,
				},
				{
					Name:          "dsport-tcp",
					ContainerPort: &tcpport,
					Protocol:      corev1.ProtocolTCP,
				},
			},
		},
	}
	return &GameServerWrapper{gs}
}

func defaultGSWithoutContainerName() *GameServerWrapper {
	gs := defaultGS().DeepCopy()
	gs.Spec.Template.Spec.Containers[0].Name = "test"
	return &GameServerWrapper{gs}
}

func defaultGSS() *GameServerSetWrapper {
	gss := &carrierv1alpha1.GameServerSet{
		Spec: carrierv1alpha1.GameServerSetSpec{
			Template: carrierv1alpha1.GameServerTemplateSpec{
				Spec: carrierv1alpha1.GameServerSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "server",
									Image: "test:latest",
								},
							},
						},
					},
				},
			},
		},
	}
	return &GameServerSetWrapper{gss}
}
