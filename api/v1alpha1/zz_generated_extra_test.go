package v1alpha1

import (
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestTencentIssuerSpecDeepCopy(t *testing.T) {
	in := &TencentIssuerSpec{Region: "r", SecretRef: SecretRef{Name: "s"}}
	out := in.DeepCopy()
	if out == in || out.Region != "r" || out.SecretRef.Name != "s" {
		t.Fatalf("unexpected copy: %#v", out)
	}
	if in.DeepCopyInto(out); out.Region != "r" {
		t.Fatal("DeepCopyInto should not clear fields")
	}
	var nilSpec *TencentIssuerSpec
	if got := nilSpec.DeepCopy(); got != nil {
		t.Fatal("nil DeepCopy should return nil")
	}
}

func TestTencentIssuerStatusDeepCopyNilConditions(t *testing.T) {
	in := &TencentIssuerStatus{}
	out := in.DeepCopy()
	if out == nil || out.Conditions != nil {
		t.Fatalf("want nil conditions, got %#v", out)
	}
	var nilStatus *TencentIssuerStatus
	if got := nilStatus.DeepCopy(); got != nil {
		t.Fatal("nil DeepCopy should return nil")
	}
}

func TestTencentIssuerStatusDeepCopyConditions(t *testing.T) {
	now := metav1.Now()
	in := &TencentIssuerStatus{Conditions: []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue, LastTransitionTime: now},
	}}
	out := in.DeepCopy()
	if len(out.Conditions) != 1 {
		t.Fatalf("want 1 condition, got %d", len(out.Conditions))
	}
	if &in.Conditions[0] == &out.Conditions[0] {
		t.Fatal("conditions must be a new slice (deep copy)")
	}
	in.Conditions[0].Reason = "mutated"
	if out.Conditions[0].Reason == "mutated" {
		t.Fatal("mutation of source must not affect copy")
	}
}

func TestTencentIssuerConditionsAccessors(t *testing.T) {
	ti := &TencentIssuer{}
	if len(ti.GetConditions()) != 0 {
		t.Fatal("empty issuer must have zero conditions")
	}
	ti.SetConditions([]metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}})
	if len(ti.GetConditions()) != 1 {
		t.Fatalf("SetConditions didn't stick: %v", ti.Status.Conditions)
	}
}

func TestTencentClusterIssuerConditionsAccessors(t *testing.T) {
	ci := &TencentClusterIssuer{}
	if len(ci.GetConditions()) != 0 {
		t.Fatal("empty cluster issuer must have zero conditions")
	}
	ci.SetConditions([]metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}})
	if len(ci.GetConditions()) != 1 {
		t.Fatalf("SetConditions didn't stick: %v", ci.Status.Conditions)
	}
}

func TestTencentClusterIssuerGVKFull(t *testing.T) {
	ci := &TencentClusterIssuer{}
	ci.SetGroupVersionKind(GroupVersion.WithKind("TencentClusterIssuer"))
	gvk := ci.GetObjectKind().GroupVersionKind()
	if gvk.Group != "tencent.cert-manager.io" {
		t.Fatalf("group: %s", gvk.Group)
	}
	if gvk.Version != "v1alpha1" {
		t.Fatalf("version: %s", gvk.Version)
	}
}

func TestTencentIssuerGVKFull(t *testing.T) {
	ti := &TencentIssuer{}
	ti.SetGroupVersionKind(GroupVersion.WithKind("TencentIssuer"))
	gvk := ti.GetObjectKind().GroupVersionKind()
	if gvk.Version != "v1alpha1" {
		t.Fatalf("version: %s", gvk.Version)
	}
}

func TestTencentIssuerDeepCopyGenerated(t *testing.T) {
	in := &TencentIssuer{
		ObjectMeta: metav1.ObjectMeta{Name: "prod", Namespace: "ns"},
		Spec: TencentIssuerSpec{
			Region:         "ap-guangzhou",
			SecretRef:      SecretRef{Name: "creds"},
			ResyncInterval: metav1.Duration{Duration: 30 * time.Minute},
		},
		Status: TencentIssuerStatus{Conditions: []metav1.Condition{{
			Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK",
		}}},
	}
	out := in.DeepCopy()
	if out == in {
		t.Fatal("DeepCopy must return new pointer")
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("DeepCopy mismatch:\nbefore=%#v\nafter=%#v", in, out)
	}
	in.Spec.Region = "changed"
	if out.Spec.Region == "changed" {
		t.Fatal("DeepCopy must not share nested spec")
	}
	in.Status.Conditions[0].Reason = "mutated"
	if out.Status.Conditions[0].Reason == "mutated" {
		t.Fatal("DeepCopy must not share status conditions")
	}
	objOut := in.DeepCopyObject()
	ti, ok := objOut.(*TencentIssuer)
	if !ok {
		t.Fatalf("DeepCopyObject must return *TencentIssuer, got %T", objOut)
	}
	if ti.Spec.Region != in.Spec.Region {
		t.Fatal("DeepCopyObject lost spec")
	}
	var nilTI *TencentIssuer
	if got := nilTI.DeepCopy(); got != nil {
		t.Fatal("nil DeepCopy should return nil")
	}
	if got := nilTI.DeepCopyObject(); got != nil {
		t.Fatal("nil DeepCopyObject should return nil")
	}
}

func TestTencentIssuerListDeepCopyGenerated(t *testing.T) {
	in := &TencentIssuerList{Items: []TencentIssuer{
		{ObjectMeta: metav1.ObjectMeta{Name: "a"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "b"}},
	}}
	out := in.DeepCopy()
	if out == in || len(out.Items) != 2 {
		t.Fatalf("DeepCopy mismatch: %v", out)
	}
	in.Items[0].Name = "mutated"
	if out.Items[0].Name == "mutated" {
		t.Fatal("Items must be a new slice")
	}
	objOut := in.DeepCopyObject()
	tl, ok := objOut.(*TencentIssuerList)
	if !ok {
		t.Fatalf("DeepCopyObject type: %T", objOut)
	}
	if len(tl.Items) != 2 {
		t.Fatal("DeepCopyObject lost items")
	}
	var nilL *TencentIssuerList
	if got := nilL.DeepCopy(); got != nil {
		t.Fatal("nil list DeepCopy should return nil")
	}
	if got := nilL.DeepCopyObject(); got != nil {
		t.Fatal("nil list DeepCopyObject should return nil")
	}
}

func TestTencentClusterIssuerDeepCopyGenerated(t *testing.T) {
	in := &TencentClusterIssuer{
		ObjectMeta: metav1.ObjectMeta{Name: "prod"},
		Spec:       TencentIssuerSpec{Region: "r", SecretRef: SecretRef{Name: "s"}},
		Status: TencentIssuerStatus{Conditions: []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionTrue},
		}},
	}
	out := in.DeepCopy()
	if out == in {
		t.Fatal("DeepCopy must return new pointer")
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatal("DeepCopy output must equal input")
	}
	in.Status.Conditions[0].Type = "mutated"
	if out.Status.Conditions[0].Type == "mutated" {
		t.Fatal("conditions must be a new slice")
	}
	objOut := in.DeepCopyObject()
	ci, ok := objOut.(*TencentClusterIssuer)
	if !ok {
		t.Fatalf("DeepCopyObject type: %T", objOut)
	}
	if ci.Spec.Region != "r" {
		t.Fatal("DeepCopyObject lost spec")
	}
	var nilCI *TencentClusterIssuer
	if got := nilCI.DeepCopy(); got != nil {
		t.Fatal("nil DeepCopy should return nil")
	}
	if got := nilCI.DeepCopyObject(); got != nil {
		t.Fatal("nil DeepCopyObject should return nil")
	}
}

func TestTencentClusterIssuerListDeepCopyGenerated(t *testing.T) {
	in := &TencentClusterIssuerList{Items: []TencentClusterIssuer{
		{ObjectMeta: metav1.ObjectMeta{Name: "a"}},
	}}
	out := in.DeepCopy()
	if out == in || len(out.Items) != 1 {
		t.Fatalf("unexpected: %v", out)
	}
	in.Items[0].Name = "mutated"
	if out.Items[0].Name == "mutated" {
		t.Fatal("items must be a new slice")
	}
	objOut := in.DeepCopyObject()
	if _, ok := objOut.(*TencentClusterIssuerList); !ok {
		t.Fatalf("DeepCopyObject type: %T", objOut)
	}
	var nilL *TencentClusterIssuerList
	if got := nilL.DeepCopy(); got != nil {
		t.Fatal("nil DeepCopy should return nil")
	}
	if got := nilL.DeepCopyObject(); got != nil {
		t.Fatal("nil DeepCopyObject should return nil")
	}
}

func TestSchemeBuilderRegistersAllTypes(t *testing.T) {
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	for _, kind := range []string{
		"TencentIssuer",
		"TencentIssuerList",
		"TencentClusterIssuer",
		"TencentClusterIssuerList",
	} {
		gvk := GroupVersion.WithKind(kind)
		if !s.Recognizes(gvk) {
			t.Fatalf("scheme must recognize %s", kind)
		}
	}
}
