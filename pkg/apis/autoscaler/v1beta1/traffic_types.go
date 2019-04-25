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

package v1beta1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TrafficSpec defines the desired state of Traffic
type TrafficSpec struct {
	// Deployment name of the deployment to intercept traffic from
	Deployment string `json:"deployment"`

	// Service name of the service associated to the deployment
	Service string `json:"service"`

	// MinReplicas minimum number of replicas to scale to when the service
	//  is activated
	MinReplicas *int `json:"minReplicas,omitempty"`

	// IdleAfter defines the duration without traffic after which we consider
	// the deployment should be scaled to zero
	IdleAfter *time.Duration `json:"idleAfter,omitempty"`
}

// TrafficStatus defines the observed state of Traffic
type TrafficStatus struct {
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Traffic is the Schema for the traffics API
// +k8s:openapi-gen=true
type Traffic struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TrafficSpec   `json:"spec,omitempty"`
	Status TrafficStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TrafficList contains a list of Traffic
type TrafficList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Traffic `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Traffic{}, &TrafficList{})
}
