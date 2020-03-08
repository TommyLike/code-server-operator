/*
Copyright 2019 tommylikehu@gmail.com.

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

package v1alpha1

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CodeServerSpec defines the desired state of CodeServer
type CodeServerSpec struct {
	// Specifies the volume size that will be used for code server
	VolumeSize string `json:"volumeSize,omitempty" protobuf:"bytes,2,opt,name=volumeSize"`
	// Specifies the storage class name for the persistent volume
	StorageClassName string `json:"storageClassName,omitempty" protobuf:"bytes,3,opt,name=storageClassName"`
	// Specifies the period before controller inactive the resource (delete all resources except volume).
	InactiveAfterSeconds *int64 `json:"inactiveAfterSeconds,omitempty" protobuf:"bytes,4,opt,name=inactiveAfterSeconds"`
	// Specifies the period before controller recycle the resource (delete all resources).
	RecycleAfterSeconds *int64 `json:"recycleAfterSeconds,omitempty" protobuf:"bytes,5,opt,name=recycleAfterSeconds"`
	// Specifies the resource requirements for code server pod.
	Resources v1.ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,6,opt,name=resources"`
	// Specifies the cipher for coder server login
	ServerCipher string `json:"serverCipher,omitempty" protobuf:"bytes,7,opt,name=serverCipher"`
	// Specifies the url for website visiting
	URL string `json:"url,omitempty" protobuf:"bytes,8,opt,name=url"`
	// Specifies the image used to running code server
	Image string `json:"image,omitempty" protobuf:"bytes,9,opt,name=image"`
	// Specifies the init plugins that will be running to finish before code server running.
	InitPlugins map[string][]string `json:"initPlugins,omitempty" protobuf:"bytes,10,opt,name=initPlugins"`
}

// ServerConditionType describes the type of state of code server condition
type ServerConditionType string

const (
	// ServerCreated means the code server has been accepted by the system.
	ServerCreated ServerConditionType = "ServerCreated"
	// ServerReady means the code server has been ready for usage.
	ServerReady ServerConditionType = "ServerReady"
	// ServerRecycled means the code server has been recycled totally.
	ServerRecycled ServerConditionType = "ServerRecycled"
	// ServerInactive means the code server will be marked inactive if `InactiveAfterSeconds` elapsed
	ServerInactive ServerConditionType = "ServerInactive"
)

// ServerCondition describes the state of the code server at a certain point.
type ServerCondition struct {
	// Type of code server condition.
	Type ServerConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}

// CodeServerStatus defines the observed state of CodeServer
type CodeServerStatus struct {
	//Server conditions
	Conditions []ServerCondition `json:"conditions,omitempty" protobuf:"bytes,1,opt,name=conditions"`
}

// +kubebuilder:object:root=true

// CodeServer is the Schema for the codeservers API
type CodeServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CodeServerSpec   `json:"spec,omitempty"`
	Status CodeServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CodeServerList contains a list of CodeServer
type CodeServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CodeServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CodeServer{}, &CodeServerList{})
}
