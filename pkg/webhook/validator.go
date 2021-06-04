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
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog"
	k8sapi "k8s.io/kubernetes/pkg/apis/core"
	k8sapiv1 "k8s.io/kubernetes/pkg/apis/core/v1"
	apicorevalidation "k8s.io/kubernetes/pkg/apis/core/validation"

	carrierv1alpha1 "github.com/ocgi/carrier/pkg/apis/carrier/v1alpha1"
)

// ValidateGameServer validates the GameServer configuration.
// If a GameServer is invalid there will be > 0 values in
// the returned array
func ValidateGameServer(gs *carrierv1alpha1.GameServer) field.ErrorList {
	errs := validateName(gs.ObjectMeta)
	errs = append(errs, validateSpec(&gs.Spec)...)
	return append(errs, validatePodTemplate(corev1.PodTemplate{Template: gs.Spec.Template})...)
}

// ValidateGameServerUpdate validate the GameServer update, only allow image now.
func ValidateGameServerUpdate(oldGS, newGS *carrierv1alpha1.GameServer) field.ErrorList {
	errs := validateName(newGS.ObjectMeta)
	// to support in-place update, allow image update
	for idx, c := range newGS.Spec.Template.Spec.Containers {
		oldGS.Spec.Template.Spec.Containers[idx].Image = c.Image
	}
	if !apiequality.Semantic.DeepEqual(oldGS.Spec.ReadinessGates, newGS.Spec.ReadinessGates) {
		errs = append(errs, field.Forbidden(field.NewPath("template.spec.readinessGates"), "readinessGates cannot be updated after creation"))
	}
	if !apiequality.Semantic.DeepEqual(oldGS.Spec.DeletableGates, newGS.Spec.DeletableGates) {
		errs = append(errs, field.Forbidden(field.NewPath("template.spec.deletableGates"), "deletableGates cannot be updated after creation"))

	}

	// allow dynamic port allocation
	for i := range oldGS.Spec.Ports {
		oldGS.Spec.Ports[i].HostPortRange = newGS.Spec.Ports[i].HostPortRange
		oldGS.Spec.Ports[i].HostPort = newGS.Spec.Ports[i].HostPort
	}

	if !apiequality.Semantic.DeepEqual(oldGS.Spec.Ports, newGS.Spec.Ports) {
		errs = append(errs, field.Forbidden(field.NewPath("template.spec.ports"), "ports cannot be updated after creation"))
	}
	// allow update object meta
	if !apiequality.Semantic.DeepEqual(oldGS.Spec.Template.Spec, newGS.Spec.Template.Spec) {
		errs = append(errs, field.Forbidden(field.NewPath("template.spec.template"), "template cannot be updated after creation"))
	}
	return errs
}

// ValidateSpec validates the GameServerSpec configuration.
func validateSpec(gss *carrierv1alpha1.GameServerSpec) field.ErrorList {
	var errs field.ErrorList
	for _, p := range gss.Ports {
		if p.ContainerPortRange != nil && p.ContainerPortRange.MinPort > p.ContainerPortRange.MaxPort {
			errs = append(errs, field.Invalid(field.NewPath("spec.ports.containerPortRange"), p.ContainerPortRange.MinPort,
				"containerPortRange.minPort can not be larger than containerPortRange.minPort"))
		}

		if p.HostPortRange != nil && p.HostPortRange.MinPort > p.HostPortRange.MaxPort {
			if p.ContainerPortRange != nil && p.ContainerPortRange.MinPort > p.ContainerPortRange.MaxPort {
				errs = append(errs, field.Invalid(field.NewPath("spec.ports.hostPortRange"), p.ContainerPortRange.MinPort,
					"hostPortRange.minPort can not be larger than hostPortRange.minPort"))
			}
		}

		if p.ContainerPortRange != nil && p.ContainerPort != nil {
			errs = append(errs, field.Forbidden(field.NewPath("spec.ports.containerPortRange"),
				"containerPortRange and ContainerPort are exclusive in one GameServer Port"))
		}

		if p.ContainerPort != nil && *p.ContainerPort <= 0 {
			errs = append(errs, field.Invalid(field.NewPath("spec.ports.containerPort"), *p.ContainerPort,
				"containerPort cannot <= 0"))
		}

		if p.ContainerPortRange != nil && p.ContainerPortRange.MinPort <= 0 {
			errs = append(errs, field.Forbidden(field.NewPath("spec.ports.containerPortRange"),
				"containerPortRange.minPort cannot <= 0"))
		}
		if p.ContainerPortRange != nil && p.ContainerPortRange.MaxPort <= 0 {
			errs = append(errs, field.Forbidden(field.NewPath("spec.ports.containerPortRange"),
				"containerPortRange.maxPort cannot <= 0"))
		}

		if p.HostPort != nil && *p.HostPort > 0 && (p.PortPolicy == carrierv1alpha1.Dynamic) {
			errs = append(errs, field.Forbidden(field.NewPath("spec.ports.hostPort"),
				"hostPort should not filled when policy is dynamic"))
		}
	}
	return append(errs, validateLabelsAndAnnotations(&gss.Template.ObjectMeta)...)
}

// validateName Check NameSize of a CRD
func validateName(c metav1.ObjectMeta) field.ErrorList {
	name := c.GetName()
	// make sure the Name of a Squad does not oversize the Label size in GSS and GS
	if len(name) > validation.LabelValueMaxLength {
		return []*field.Error{field.TooLong(field.NewPath("name"), len(name), validation.LabelValueMaxLength)}
	}
	return nil
}

// validateLabelsAndAnnotations validates the labels annotations.
func validateLabelsAndAnnotations(objMeta *metav1.ObjectMeta) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, metav1validation.ValidateLabels(objMeta.Labels, field.NewPath("labels"))...)
	allErrs = append(allErrs, apivalidation.ValidateAnnotations(objMeta.Annotations, field.NewPath("annotations"))...)
	return allErrs
}

// ValidateGameServerSetUpdate validate the GameServerSet update, only allow image, pullPolicy and replicas now.
func ValidateGameServerSetUpdate(oldGSS, newGSS *carrierv1alpha1.GameServerSet) field.ErrorList {
	errs := validateName(newGSS.ObjectMeta)
	// to support in-place update, allow image update
	for idx, c := range oldGSS.Spec.Template.Spec.Template.Spec.Containers {
		newGSS.Spec.Template.Spec.Template.Spec.Containers[idx].Image = c.Image
		newGSS.Spec.Template.Spec.Template.Spec.Containers[idx].ImagePullPolicy = c.ImagePullPolicy
	}
	// allow update replicas
	newGSS.Spec.Replicas = oldGSS.Spec.Replicas
	// allow update annotation and labels
	if !apiequality.Semantic.DeepEqual(oldGSS.Spec.Template.Spec, newGSS.Spec.Template.Spec) {
		errs = append(errs, field.Forbidden(field.NewPath("spec.template.spec"), "GameServer Spec are not allowed to changed, expect for image and resource"))
	}

	return errs
}

// ValidateGameServerSet validates when Create occurs, check name, label, annotaions and podSpec
func ValidateGameServerSet(gsSet *carrierv1alpha1.GameServerSet) field.ErrorList {
	errs := validateName(gsSet.ObjectMeta)
	errs = append(errs, validateSpec(&gsSet.Spec.Template.Spec)...)
	errs = append(errs, validateLabelsAndAnnotations(&gsSet.Spec.Template.ObjectMeta)...)
	return append(errs, validatePodTemplate(corev1.PodTemplate{ObjectMeta: gsSet.ObjectMeta, Template: gsSet.Spec.Template.Spec.Template})...)
}

// ValidateSquad validates when Create occurs, check name, label, annotaions and podSpec
func ValidateSquad(squad *carrierv1alpha1.Squad) field.ErrorList {
	errs := validateName(squad.ObjectMeta)
	errs = append(errs, validateSpec(&squad.Spec.Template.Spec)...)
	errs = append(errs, validateLabelsAndAnnotations(&squad.Spec.Template.ObjectMeta)...)
	return append(errs, validatePodTemplate(corev1.PodTemplate{ObjectMeta: squad.ObjectMeta, Template: squad.Spec.Template.Spec.Template})...)
}

// ValidateSquadUpdate validate the Squad update, only allow image, pullPolicy for pod spec.
// other fields to controller update policy are all alowed
func ValidateSquadUpdate(oldSquad, newSquad *carrierv1alpha1.Squad) field.ErrorList {
	errs := validateName(newSquad.ObjectMeta)
	// to support in-place update, allow image update
	for idx, c := range oldSquad.Spec.Template.Spec.Template.Spec.Containers {
		newSquad.Spec.Template.Spec.Template.Spec.Containers[idx].Image = c.Image
		newSquad.Spec.Template.Spec.Template.Spec.Containers[idx].ImagePullPolicy = c.ImagePullPolicy
	}
	if !apiequality.Semantic.DeepEqual(oldSquad.Spec.Template.Spec, oldSquad.Spec.Template.Spec) {
		errs = append(errs, field.Forbidden(field.NewPath("spec.template.spec"), "GameServer Spec are not allowed to changed, expect for image and resource"))
	}

	return errs
}

func validatePodTemplate(specTemplate corev1.PodTemplate) field.ErrorList {
	copy := specTemplate.DeepCopy()
	coreTemp := &k8sapi.PodTemplate{}
	copy.Namespace = "fake"
	copy.Name = "fake"
	k8sapiv1.SetObjectDefaults_PodTemplate(copy)
	err := k8sapiv1.Convert_v1_PodTemplate_To_core_PodTemplate(copy, coreTemp, nil)
	if err != nil {
		klog.Errorf(err.Error())
		return []*field.Error{field.InternalError(field.NewPath("PodTemplate"), err)}
	}

	return apicorevalidation.ValidatePodTemplate(coreTemp)
}
