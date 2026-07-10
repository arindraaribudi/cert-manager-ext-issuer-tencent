package controller

import (
	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
)

const (
	AnnotationCertID     = "tencent.cert-manager.io/certificate-id"
	AnnotationForceSync  = "tencent.cert-manager.io/force-sync"
	AnnotationForceRenew = "cert-manager.io/force-renew"
)

func CertIDFromCertificate(cert *cmapi.Certificate) (string, bool) {
	v, ok := cert.Annotations[AnnotationCertID]
	return v, ok && v != ""
}

func IsForceSyncSet(cert *cmapi.Certificate) bool {
	return cert.Annotations[AnnotationForceSync] == "true"
}
