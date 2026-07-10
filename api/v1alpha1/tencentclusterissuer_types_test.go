package v1alpha1

import "testing"

func TestTencentClusterIssuerGVK(t *testing.T) {
	ci := &TencentClusterIssuer{}
	ci.SetGroupVersionKind(GroupVersion.WithKind("TencentClusterIssuer"))
	gvk := ci.GetObjectKind().GroupVersionKind()
	if gvk.Kind != "TencentClusterIssuer" {
		t.Fatalf("unexpected kind: %s", gvk.Kind)
	}
	if gvk.Version != "v1alpha1" {
		t.Fatalf("unexpected version: %s", gvk.Version)
	}
}
