// ponytail: controller-gen 0.21.0 against Go 1.26 / apimachinery 0.36.2 is not emitting
// DeepCopy/DeepCopyInto for nested struct types whose only deep-copyable fields are
// slices of third-party types with their own DeepCopy. Re-run make generate after upgrading
// controller-gen; remove this file once zz_generated.deepcopy.go gains these methods.
package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func (in *TencentIssuerSpec) DeepCopyInto(out *TencentIssuerSpec) {
	*out = *in
}

func (in *TencentIssuerSpec) DeepCopy() *TencentIssuerSpec {
	if in == nil {
		return nil
	}
	out := new(TencentIssuerSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *TencentIssuerStatus) DeepCopyInto(out *TencentIssuerStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *TencentIssuerStatus) DeepCopy() *TencentIssuerStatus {
	if in == nil {
		return nil
	}
	out := new(TencentIssuerStatus)
	in.DeepCopyInto(out)
	return out
}
