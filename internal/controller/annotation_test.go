package controller

import (
	"testing"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCertIDFromCertificate(t *testing.T) {
	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{AnnotationCertID: "abc123"}},
	}
	id, ok := CertIDFromCertificate(cert)
	if !ok || id != "abc123" {
		t.Fatalf("want abc123, got %s ok=%v", id, ok)
	}
}

func TestCertIDFromCertificateMissing(t *testing.T) {
	cert := &cmapi.Certificate{}
	if id, ok := CertIDFromCertificate(cert); ok || id != "" {
		t.Fatalf("want empty/false, got %q/%v", id, ok)
	}
}

func TestCertIDFromCertificateEmpty(t *testing.T) {
	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{AnnotationCertID: ""}},
	}
	if id, ok := CertIDFromCertificate(cert); ok || id != "" {
		t.Fatalf("want empty/false for empty value, got %q/%v", id, ok)
	}
}

func TestForceSync(t *testing.T) {
	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{AnnotationForceSync: "true"}},
	}
	if !IsForceSyncSet(cert) {
		t.Fatal("expected force sync true")
	}
}

func TestForceSyncNotTrue(t *testing.T) {
	for _, v := range []string{"", "false", "TRUE", "1"} {
		cert := &cmapi.Certificate{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{AnnotationForceSync: v}},
		}
		if IsForceSyncSet(cert) {
			t.Fatalf("want false for %q", v)
		}
	}
}

func TestForceSyncMissing(t *testing.T) {
	cert := &cmapi.Certificate{}
	if IsForceSyncSet(cert) {
		t.Fatal("want false when annotation absent")
	}
}

func TestAnnotationCertIDPredicate(t *testing.T) {
	with := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{AnnotationCertID: "x"}},
	}
	without := &cmapi.Certificate{ObjectMeta: metav1.ObjectMeta{Name: "n"}}
	if !hasCertIDAnnotation(with) {
		t.Fatal("with annotation must be selected")
	}
	if hasCertIDAnnotation(without) {
		t.Fatal("without annotation must be filtered out")
	}
}
