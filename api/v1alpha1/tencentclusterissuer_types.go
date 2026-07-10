package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
type TencentClusterIssuer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TencentIssuerSpec   `json:"spec,omitempty"`
	Status            TencentIssuerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type TencentClusterIssuerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TencentClusterIssuer `json:"items"`
}

func (ci *TencentClusterIssuer) GetConditions() []metav1.Condition  { return ci.Status.Conditions }
func (ci *TencentClusterIssuer) SetConditions(c []metav1.Condition) { ci.Status.Conditions = c }

var _ runtime.Object = (*TencentClusterIssuer)(nil)
