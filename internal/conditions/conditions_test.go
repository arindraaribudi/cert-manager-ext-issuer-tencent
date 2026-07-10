package conditions

import (
	"testing"
	"time"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type stub struct{ c []metav1.Condition }

func (s *stub) GetConditions() []metav1.Condition  { return s.c }
func (s *stub) SetConditions(c []metav1.Condition) { s.c = c }

func TestSetReadyAndSynced(t *testing.T) {
	s := &stub{}
	SetReady(s, metav1.ConditionTrue, "OK", "ok")
	SetSynced(s, metav1.ConditionTrue, "OK", "ok")
	if len(s.c) != 2 {
		t.Fatalf("want 2 conditions, got %d", len(s.c))
	}
}

func TestConditionReasonMapping(t *testing.T) {
	cases := []struct {
		fn     func(Status, metav1.ConditionStatus, string, string)
		typ    string
		reason string
	}{
		{SetReady, "Ready", "AuthError"},
		{SetSynced, "Synced", "RemoteChanged"},
	}
	for _, tc := range cases {
		s := &stub{}
		tc.fn(s, metav1.ConditionFalse, tc.reason, "msg")
		if s.c[0].Type != tc.typ || s.c[0].Reason != tc.reason {
			t.Fatalf("got %#v", s.c[0])
		}
	}
}

func TestSetCertificateConditionNew(t *testing.T) {
	c := &cmapi.Certificate{}
	SetCertificateCondition(c, cmmeta.ConditionTrue, "Synced", "ok")
	if len(c.Status.Conditions) != 1 {
		t.Fatalf("want 1 condition, got %d", len(c.Status.Conditions))
	}
	got := c.Status.Conditions[0]
	if got.Type != cmapi.CertificateConditionReady || got.Status != cmmeta.ConditionTrue || got.Reason != "Synced" {
		t.Fatalf("unexpected condition: %#v", got)
	}
	if got.LastTransitionTime == nil {
		t.Fatal("LastTransitionTime must be set on add")
	}
}

func TestSetCertificateConditionSameStatusKeepsTransitionTime(t *testing.T) {
	earlier := metav1.NewTime(time.Now().Add(-time.Hour))
	c := &cmapi.Certificate{
		Status: cmapi.CertificateStatus{
			Conditions: []cmapi.CertificateCondition{{
				Type:               cmapi.CertificateConditionReady,
				Status:             cmmeta.ConditionTrue,
				Reason:             "Synced",
				LastTransitionTime: &earlier,
			}},
		},
	}
	SetCertificateCondition(c, cmmeta.ConditionTrue, "Synced", "again")
	if !c.Status.Conditions[0].LastTransitionTime.Equal(&earlier) {
		t.Fatalf("transition time must be preserved on same-status update")
	}
}

func TestSetCertificateConditionStatusChangeResetsTransitionTime(t *testing.T) {
	earlier := metav1.NewTime(time.Now().Add(-time.Hour))
	c := &cmapi.Certificate{
		Status: cmapi.CertificateStatus{
			Conditions: []cmapi.CertificateCondition{{
				Type:               cmapi.CertificateConditionReady,
				Status:             cmmeta.ConditionTrue,
				LastTransitionTime: &earlier,
			}},
		},
	}
	SetCertificateCondition(c, cmmeta.ConditionFalse, "Synced", "failed")
	if c.Status.Conditions[0].LastTransitionTime.Equal(&earlier) {
		t.Fatal("transition time must update on status change")
	}
}

func TestSetCertificateConditionKeepsOtherConditions(t *testing.T) {
	other := cmapi.CertificateCondition{Type: "NonReady", Status: cmmeta.ConditionTrue, Reason: "X"}
	c := &cmapi.Certificate{Status: cmapi.CertificateStatus{Conditions: []cmapi.CertificateCondition{other}}}
	SetCertificateCondition(c, cmmeta.ConditionTrue, "Synced", "ok")
	if len(c.Status.Conditions) != 2 {
		t.Fatalf("want 2 conditions, got %d", len(c.Status.Conditions))
	}
	if c.Status.Conditions[0].Type != "NonReady" {
		t.Fatal("non-Ready condition must be untouched")
	}
}
