package conditions

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
)

type Status interface {
	GetConditions() []metav1.Condition
	SetConditions([]metav1.Condition)
}

func SetReady(s Status, status metav1.ConditionStatus, reason, message string) {
	set(s, "Ready", status, reason, message)
}

func SetSynced(s Status, status metav1.ConditionStatus, reason, message string) {
	set(s, "Synced", status, reason, message)
}

func set(s Status, typ string, status metav1.ConditionStatus, reason, message string) {
	conds := s.GetConditions()
	meta.SetStatusCondition(&conds, metav1.Condition{
		Type: typ, Status: status, Reason: reason, Message: message,
	})
	s.SetConditions(conds)
}

// SetCertificateCondition sets the Ready condition on a cert-manager Certificate.
// Separate from SetReady because cmapi.Certificate uses cmapi.CertificateCondition
// (cert-manager's local meta/v1 types), so the generic Status interface doesn't apply. // ponytail: separate helper, cmapi types differ from metav1
func SetCertificateCondition(c *cmapi.Certificate, status cmmeta.ConditionStatus, reason, message string) {
	now := metav1.Now()
	cond := cmapi.CertificateCondition{
		Type:               cmapi.CertificateConditionReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: &now,
	}
	for i, existing := range c.Status.Conditions {
		if existing.Type == cond.Type {
			cond.LastTransitionTime = existing.LastTransitionTime
			if existing.Status != cond.Status {
				cond.LastTransitionTime = &now
			}
			c.Status.Conditions[i] = cond
			return
		}
	}
	c.Status.Conditions = append(c.Status.Conditions, cond)
}
