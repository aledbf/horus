/*

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

package validating

import (
	"context"
	"fmt"
	"net/http"

	autoscalerv1beta1 "github.com/aledbf/horus/pkg/apis/autoscaler/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

func init() {
	webhookName := "validating-create-update-traffic"
	if HandlerMap[webhookName] == nil {
		HandlerMap[webhookName] = []admission.Handler{}
	}
	HandlerMap[webhookName] = append(HandlerMap[webhookName], &TrafficCreateUpdateHandler{})
}

// TrafficCreateUpdateHandler handles Traffic
type TrafficCreateUpdateHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder types.Decoder
}

func (h *TrafficCreateUpdateHandler) validatingTrafficFn(ctx context.Context, obj *autoscalerv1beta1.Traffic) (bool, string, error) {
	if obj.Namespace == "" {
		obj.Namespace = metav1.NamespaceDefault
	}

	deployment := &appsv1.Deployment{}
	err := h.Client.Get(context.Background(), apitypes.NamespacedName{
		Name:      obj.Spec.Deployment,
		Namespace: obj.Namespace,
	}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, fmt.Sprintf("deployment %v/%v does not exists", obj.Namespace, obj.Spec.Deployment), nil
		}

		return false, "unexpected error", err
	}

	service := &corev1.Service{}
	err = h.Client.Get(context.Background(), apitypes.NamespacedName{
		Name:      obj.Spec.Service,
		Namespace: obj.Namespace,
	}, service)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, fmt.Sprintf("service %v/%v does not exists", obj.Namespace, obj.Spec.Service), nil
		}

		return false, "unexpected error", err
	}

	trafficList := &autoscalerv1beta1.TrafficList{}
	err = h.Client.List(context.Background(), &client.ListOptions{
		Namespace: obj.Namespace,
	}, trafficList)
	if err != nil {
		return false, "unexpected error", err

	}

	for _, traffic := range trafficList.Items {
		if traffic.Name == obj.Name {
			continue
		}

		if traffic.Spec.Deployment == traffic.Spec.Deployment &&
			traffic.Spec.Service == traffic.Spec.Service {
			return false, "Already exist a definition for the same deployment and service", nil
		}

		if traffic.Spec.Service == traffic.Spec.Service {
			return false, fmt.Sprintf("Already exist a definition using the service %v (%v)", obj.Spec.Service, traffic.Name), nil
		}
	}

	return true, "allowed to be admitted", nil
}

var _ admission.Handler = &TrafficCreateUpdateHandler{}

// Handle handles admission requests.
func (h *TrafficCreateUpdateHandler) Handle(ctx context.Context, req types.Request) types.Response {
	obj := &autoscalerv1beta1.Traffic{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	allowed, reason, err := h.validatingTrafficFn(ctx, obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}
	return admission.ValidationResponse(allowed, reason)
}

var _ inject.Client = &TrafficCreateUpdateHandler{}

// InjectClient injects the client into the TrafficCreateUpdateHandler
func (h *TrafficCreateUpdateHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ inject.Decoder = &TrafficCreateUpdateHandler{}

// InjectDecoder injects the decoder into the TrafficCreateUpdateHandler
func (h *TrafficCreateUpdateHandler) InjectDecoder(d types.Decoder) error {
	h.Decoder = d
	return nil
}
