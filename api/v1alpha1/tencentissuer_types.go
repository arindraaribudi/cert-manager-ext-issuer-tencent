// +groupName=tencent.cert-manager.io
// +versionName=v1alpha1
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

type TencentIssuerSpec struct {
	Region         string          `json:"region"`
	SecretRef      SecretRef       `json:"secretRef,omitempty"`
	Endpoint       string          `json:"endpoint,omitempty"`
	ResyncInterval metav1.Duration `json:"resyncInterval,omitempty"`
}

// SecretRef optionally references a k8s Secret with static Tencent Cloud
// credentials (secret-id + secret-key). Leave Name empty to authenticate via
// the CVM instance role / pod-identity metadata service
// (metadata.tencentyun.com). Namespace defaults to the Certificate's namespace
// when omitted.
type SecretRef struct {
	// ponytail: empty secretRef → pod-identity fallback; static path stays default.
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type TencentIssuerStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type TencentIssuer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TencentIssuerSpec   `json:"spec,omitempty"`
	Status            TencentIssuerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type TencentIssuerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TencentIssuer `json:"items"`
}

func (ti *TencentIssuer) GetConditions() []metav1.Condition  { return ti.Status.Conditions }
func (ti *TencentIssuer) SetConditions(c []metav1.Condition) { ti.Status.Conditions = c }

var _ runtime.Object = (*TencentIssuer)(nil)
