/*
Copyright 2025.

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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// S3TargetSpec defines the desired state of S3Target
type S3TargetSpec struct {
	Bucket   string `json:"bucket,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	// +optional
	Region string `json:"region,omitempty"`
	// +optional
	AccessKey string `json:"accessKey,omitempty"`
	// +optional
	SecretKey string            `json:"secretKey,omitempty"`
	Binaries  map[string]string `json:"binaries,omitempty"`
	SizeLimit resource.Quantity `json:"sizeLimit,omitempty"`
}

// S3TargetStatus defines the observed state of S3Target.
type S3TargetStatus struct {
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// S3Target is the Schema for the s3targets API
type S3Target struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of S3Target
	// +required
	Spec S3TargetSpec `json:"spec"`

	// status defines the observed state of S3Target
	// +optional
	Status S3TargetStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// S3TargetList contains a list of S3Target
type S3TargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []S3Target `json:"items"`
}

func init() {
	SchemeBuilder.Register(&S3Target{}, &S3TargetList{})
}
