package v1alpha1

import (
	"testing"
)

func TestTencentIssuerGVK(t *testing.T) {
	ti := &TencentIssuer{}
	ti.SetGroupVersionKind(GroupVersion.WithKind("TencentIssuer"))
	gvk := ti.GetObjectKind().GroupVersionKind()
	if gvk.Group != "tencent.cert-manager.io" {
		t.Fatalf("unexpected group: %s", gvk.Group)
	}
}
