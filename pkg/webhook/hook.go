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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/listers/core/v1"
	rbaclisterv1 "k8s.io/client-go/listers/rbac/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/ocgi/carrier-webhook/pkg/util"
	"github.com/ocgi/carrier/pkg/apis/carrier"
	"github.com/ocgi/carrier/pkg/apis/carrier/v1alpha1"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
)

const (
	LBReadyKey                = "externalnetwork.ocgi.dev/lb-ready"
	ExternalNetworkKey        = "carrier.ocgi.dev/external-network-type"
	grpcPortKey               = "carrier.ocgi.dev/grpc-port"
	httpPortKey               = "carrier.ocgi.dev/http-port"
	sdkserverSidecarName      = "carrier-gameserver-sidecar"
	gsEnvKey                  = "GAMESERVER_NAME"
	grpcPortEnv               = "CARRIER_SDK_GRPC_PORT"
	httpPortEnv               = "CARRIER_SDK_HTTP_PORT"
	nsKey                     = "POD_NAMESPACE"
	mountPath                 = "/var/run/secrets/kubernetes.io/serviceaccount"
	defaultServiceAccountName = "carrier-sdk"
	defaultClusterRoleName    = "carrier-sdk"
	defaultRoleBingName       = "carrier-sdk"
)

// SideCarConfig describes the config of sidecar
type SideCarConfig struct {
	// Image describes the image version
	Image string
	// CPU if cpu config of side car
	CPU resource.Quantity
	// Memory if memory config of side car
	Memory resource.Quantity
	// HttpPort is the port for http
	HttpPort int
	// GrpcPort is the port for grpc
	GrpcPort int
}

type webhookServer struct {
	*http.Server
	config            *SideCarConfig
	saLister          v1.ServiceAccountLister
	roleBindingLister rbaclisterv1.RoleBindingLister
	saSynced          cache.InformerSynced
	roleBindingSynced cache.InformerSynced
	kubeClient        kubernetes.Interface
}

func init() {
	_ = corev1.AddToScheme(runtimeScheme)
	_ = admissionregistrationv1beta1.AddToScheme(runtimeScheme)
	runtimeScheme.AddKnownTypes(v1alpha1.SchemeGroupVersion,
		&v1alpha1.GameServer{}, &v1alpha1.GameServerSet{}, &v1alpha1.Squad{})
}

// NewWebhookServer creates a new server
func NewWebhookServer(config *SideCarConfig, kubeClient kubernetes.Interface,
	factory informers.SharedInformerFactory) *webhookServer {
	saInformer := factory.Core().V1().ServiceAccounts()
	roleBindingInformer := factory.Rbac().V1().RoleBindings()
	return &webhookServer{
		config:            config,
		saLister:          saInformer.Lister(),
		roleBindingLister: roleBindingInformer.Lister(),
		kubeClient:        kubeClient,
		saSynced:          saInformer.Informer().HasSynced,
		roleBindingSynced: roleBindingInformer.Informer().HasSynced,
	}
}

// WaitForCacheSynced wait the cache synced or die
func (whsvr *webhookServer) WaitForCacheSynced(stop <-chan struct{}) {
	klog.V(4).Info("Wait for cache sync")
	if !cache.WaitForCacheSync(stop, whsvr.saSynced, whsvr.roleBindingSynced) {
		klog.Fatal("Sync cache failed")
	}
	if err := whsvr.createDefaultClusterRole(); err != nil {
		klog.Fatalf("Create default cluster role failed: %v", err)
	}
}

// Serve method for webhook server
func (whsvr *webhookServer) Serve(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	klog.V(6).Infof("Receive request: %+v", *r)
	if len(body) == 0 {
		klog.Error("empty body")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		klog.Errorf("Content-Type=%s, expect application/json", contentType)
		http.Error(w, "invalid Content-Type, expect `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	var admissionResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		klog.Errorf("Can't decode body: %v", err)
		admissionResponse = &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	} else {
		if r.URL.Path == "/mutate" {
			admissionResponse = whsvr.mutate(&ar)
		}
	}

	admissionReview := v1beta1.AdmissionReview{}
	if admissionResponse != nil {
		admissionReview.Response = admissionResponse
		if ar.Request != nil {
			admissionReview.Response.UID = ar.Request.UID
		}
	}

	resp, err := json.Marshal(admissionReview)
	if err != nil {
		klog.Errorf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}
	if _, err := w.Write(resp); err != nil {
		klog.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

// mutate will validate and mutate GameSerer, GameServerSet, Squad
func (whsvr *webhookServer) mutate(ar *v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	req := ar.Request

	klog.Infof("AdmissionReview for Kind=%v, Namespace=%v Name=%v UID=%v Operation=%v UserInfo=%v",
		req.Kind, req.Namespace, req.Name, req.UID, req.Operation, req.UserInfo)
	var err error
	var patch []byte
	var el field.ErrorList
	switch req.Kind.Kind {
	case "GameServer":
		patch, el, err = whsvr.forGameServer(req)
	case "GameServerSet":
		patch, el, err = whsvr.forGameServerSet(req)
	case "Squad":
		patch, el, err = whsvr.forSquad(req)
	case "Pod":
		patch, el, err = forPod(req, whsvr.config)
	}
	if len(patch) != 0 {
		klog.V(6).Infof("Final patch %+v", string(patch))
	}
	result := metav1.Status{
		Details: &metav1.StatusDetails{
			Name:  ar.Request.Name,
			Group: ar.Request.Kind.Group,
			Kind:  ar.Request.Kind.Kind,
			UID:   ar.Request.UID,
		},
	}
	if err != nil {
		klog.Error(err)
		result.Code = 400
		result.Message = err.Error()
		finalErr := errors.NewInvalid(schema.GroupKind{Group: carrier.GroupName, Kind: ar.Kind}, ar.Request.Name, el)
		result.Details.Causes = finalErr.ErrStatus.Details.Causes
		return &v1beta1.AdmissionResponse{
			Allowed: false,
			Result:  &result,
		}
	}
	jsonPatch := v1beta1.PatchTypeJSONPatch
	return &v1beta1.AdmissionResponse{
		Allowed:   true,
		Result:    &result,
		Patch:     patch,
		PatchType: &jsonPatch,
	}
}

func (whsvr *webhookServer) createDefaultClusterRole() error {
	_, err := whsvr.kubeClient.RbacV1().ClusterRoles().Create(defaultClusterRole())
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (whsvr *webhookServer) createSA(namespace string, saName string) error {
	if saName != "" && saName != defaultServiceAccountName {
		return nil
	}
	_, err := whsvr.saLister.ServiceAccounts(namespace).Get(defaultServiceAccountName)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if errors.IsNotFound(err) {
		_, err = whsvr.kubeClient.CoreV1().ServiceAccounts(namespace).Create(defaultServiceAccount(namespace))
		if err != nil && !errors.IsAlreadyExists(err) {
			return err
		}
	}
	_, err = whsvr.roleBindingLister.RoleBindings(namespace).Get(defaultRoleBingName)
	if err == nil && !errors.IsNotFound(err) {
		return err
	}
	if errors.IsNotFound(err) {
		_, err = whsvr.kubeClient.RbacV1().RoleBindings(namespace).Create(defaultRoleBinding(namespace))
		if err != nil && !errors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func (whsvr *webhookServer) forSquad(req *v1beta1.AdmissionRequest) ([]byte, field.ErrorList, error) {
	var squad, oldSquad v1alpha1.Squad
	if err := json.Unmarshal(req.Object.Raw, &squad); err != nil {
		klog.Errorf("Could not unmarshal raw object: %v", err)
		return nil, nil, err
	}
	if err := whsvr.createSA(req.Namespace, squad.Spec.Template.Spec.Template.Spec.ServiceAccountName); err != nil {
		klog.Errorf("Could create service account: %v", err)
		return nil, nil, err
	}
	if req.Operation == v1beta1.Create {
		newSquad := EnsureDefaultsForSquad(&squad)
		// validate
		errs := ValidateSquad(newSquad)
		if len(errs) != 0 {
			return nil, errs, errs.ToAggregate()
		}
		// mutate
		patch, err := util.CreateJsonPatch(squad, newSquad)
		return patch, nil, err
	}

	if req.Operation == v1beta1.Update {
		if err := json.Unmarshal(req.OldObject.Raw, &oldSquad); err != nil {
			klog.Errorf("Could not unmarshal raw object: %v", err)
			return nil, nil, err
		}
		// validate
		errs := ValidateSquadUpdate(&oldSquad, &squad)
		if len(errs) != 0 {
			return nil, errs, errs.ToAggregate()
		}
	}
	return nil, nil, nil
}

func (whsvr *webhookServer) forGameServerSet(req *v1beta1.AdmissionRequest) ([]byte, field.ErrorList, error) {
	var gameServerSet, oldGameServerSet v1alpha1.GameServerSet
	if err := json.Unmarshal(req.Object.Raw, &gameServerSet); err != nil {
		klog.Errorf("Could not unmarshal raw object: %v", err)
		return nil, nil, err
	}
	if err := whsvr.createSA(req.Namespace,
		gameServerSet.Spec.Template.Spec.Template.Spec.ServiceAccountName); err != nil {
		klog.Errorf("Could create service account: %v", err)
		return nil, nil, err
	}
	if req.Operation == v1beta1.Create {
		newGameServerSet := EnsureDefaultsForGameServerSet(&gameServerSet)
		// validate
		errs := ValidateGameServerSet(newGameServerSet)
		if len(errs) != 0 {
			return nil, errs, errs.ToAggregate()
		}
		patch, err := util.CreateJsonPatch(gameServerSet, newGameServerSet)
		return patch, nil, err
	}
	if req.Operation == v1beta1.Update {
		if err := json.Unmarshal(req.OldObject.Raw, &oldGameServerSet); err != nil {
			klog.Errorf("Could not unmarshal old raw object: %v", err)
			return nil, nil, err
		}
		// validate
		errs := ValidateGameServerSetUpdate(&oldGameServerSet, &gameServerSet)
		if len(errs) != 0 {
			return nil, errs, errs.ToAggregate()
		}
	}
	return nil, nil, nil
}

func (whsvr *webhookServer) forGameServer(req *v1beta1.AdmissionRequest) ([]byte, field.ErrorList, error) {
	var gameSvr, oldGameSvr v1alpha1.GameServer
	if err := json.Unmarshal(req.Object.Raw, &gameSvr); err != nil {
		klog.Errorf("Could not unmarshal raw object: %v", err)
		return nil, nil, err
	}
	if err := whsvr.createSA(req.Namespace, gameSvr.Spec.Template.Spec.ServiceAccountName); err != nil {
		klog.Errorf("Could create service account: %v", err)
		return nil, nil, err
	}
	if req.Operation == v1beta1.Create {
		newGameServer := EnsureDefaultForGameServer(&gameSvr)
		// validate
		errs := ValidateGameServer(newGameServer)
		if len(errs) != 0 {
			return nil, errs, errs.ToAggregate()
		}
		patch, err := util.CreateJsonPatch(gameSvr, newGameServer)
		return patch, nil, err
	}

	if req.Operation == v1beta1.Update {
		if err := json.Unmarshal(req.OldObject.Raw, &oldGameSvr); err != nil {
			klog.Errorf("Could not unmarshal raw object: %v", err)
			return nil, nil, err
		}
		// validate
		errs := ValidateGameServerUpdate(&oldGameSvr, &gameSvr)
		if len(errs) != 0 {
			return nil, errs, errs.ToAggregate()
		}
	}
	return nil, nil, nil
}

func defaultClusterRole() *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultClusterRoleName,
		},
		Rules: []rbacv1.PolicyRule{
			// events
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create", "patch"},
			},
			// carrier.cogi.dev
			{
				APIGroups: []string{"carrier.ocgi.dev"},
				Resources: []string{"gameservers", "gameservers/status", "webhookconfigurations"},
				Verbs:     []string{"*"},
			},
		},
	}
}

func defaultRoleBinding(namespace string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultRoleBingName,
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     defaultClusterRoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      defaultServiceAccountName,
				Namespace: namespace,
			},
		},
	}
}

func defaultServiceAccount(namespace string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultServiceAccountName,
			Namespace: namespace,
		},
	}
}

func forPod(req *v1beta1.AdmissionRequest, config *SideCarConfig) ([]byte, field.ErrorList, error) {
	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		klog.Errorf("Could not unmarshal raw object: %v", err)
		return nil, nil, err
	}

	if req.Operation == v1beta1.Create {
		// validate
		opts := []option{
			WithImageName(config),
			WithHealthCheck(),
			WithEnvs(&pod),
		}
		if !config.CPU.IsZero() || !config.Memory.IsZero() {
			opts = append(opts, WithResource(config))
		}
		httpPort, grpcPort := getPorts(config, &pod)
		addPortEnv := func(pod *corev1.Pod) {
			for i, c := range pod.Spec.Containers {
				if c.Name == sdkserverSidecarName {
					continue
				}
				envs := []corev1.EnvVar{
					{
						Name:  grpcPortEnv,
						Value: strconv.Itoa(grpcPort),
					},
					{
						Name:  httpPortEnv,
						Value: strconv.Itoa(httpPort),
					},
				}
				pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, envs...)
			}
		}
		opts = append(opts, WithArgs(httpPort, grpcPort))
		podCopy := EnsurePod(&pod, addPortEnv, opts...)
		patch, err := util.CreateJsonPatch(pod, podCopy)

		return patch, nil, err
	}
	return nil, nil, nil
}

func getPorts(config *SideCarConfig, pod *corev1.Pod) (int, int) {
	httpPort, grpcPort := config.HttpPort, config.GrpcPort
	if pod.Annotations == nil {
		return httpPort, grpcPort
	}
	gprcStr, ok := pod.Annotations[grpcPortKey]
	if ok {
		grpcPortCustom, err := strconv.Atoi(gprcStr)
		if err == nil {
			grpcPort = grpcPortCustom
		}
	}
	httpStr, ok := pod.Annotations[httpPortKey]
	if ok {
		httpPortCustom, err := strconv.Atoi(httpStr)
		if err == nil {
			httpPort = httpPortCustom
		}
	}
	return httpPort, grpcPort
}
