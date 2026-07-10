package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/tencentcloud/tencentcloud-sdk-go-intl-en/tencentcloud/common"

	api "github.com/arindraaribudi/cert-manager-ext-issuer-tencent/api/v1alpha1"
	"github.com/arindraaribudi/cert-manager-ext-issuer-tencent/internal/tencent"
	"github.com/arindraaribudi/cert-manager-ext-issuer-tencent/internal/tencent/fake"
)

const (
	leafPEM  = "-----BEGIN CERTIFICATE-----\nMIIBleaf\n-----END CERTIFICATE-----\n"
	chainPEM = "-----BEGIN CERTIFICATE-----\nMIIBchain\n-----END CERTIFICATE-----\n"
	keyPEM   = "-----BEGIN PRIVATE KEY-----\nMIIBkey\n-----END PRIVATE KEY-----\n"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := api.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := cmapi.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func newReq(name, ns string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: ns}}
}

// TestReconcileWritesSecret exercises the Certificate reconciler end-to-end:
// annotation triggers it, the Tencent fake yields cert+key, both land in the
// target Secret as tls.crt/tls.key. // ponytail: one test covers the user requirement
// (Secret must hold both tls.crt and tls.key), add cases for failure paths when
// real flows surface them.
func TestReconcileWritesSecret(t *testing.T) {
	issuer := &api.TencentIssuer{
		ObjectMeta: metav1.ObjectMeta{Name: "prod", Namespace: "ns"},
		Spec:       api.TencentIssuerSpec{Region: "ap-guangzhou", SecretRef: api.SecretRef{Name: "creds"}},
	}
	creds := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "ns"},
		Data:       map[string][]byte{"secret-id": []byte("id"), "secret-key": []byte("key")},
	}
	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my",
			Namespace:   "ns",
			Annotations: map[string]string{AnnotationCertID: "abc"},
		},
		Spec: cmapi.CertificateSpec{
			SecretName: "my-tls",
			IssuerRef: cmmeta.IssuerReference{
				Name:  "prod",
				Group: "tencent.cert-manager.io",
				Kind:  "TencentIssuer",
			},
		},
	}

	cli := ctrlfake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(issuer, creds, cert).
		WithStatusSubresource(&cmapi.Certificate{}, &api.TencentIssuer{}).
		Build()

	fc := fake.New(fake.WithCertificate("abc", leafPEM, chainPEM, keyPEM))
	r := &IssuerReconciler{
		Client: cli,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fc, nil
		},
	}

	res, err := r.Reconcile(context.Background(), newReq("my", "ns"))
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter > 0 {
		t.Fatal("unexpected requeue")
	}

	var got corev1.Secret
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "my-tls", Namespace: "ns"}, &got); err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if len(got.Data[corev1.TLSCertKey]) == 0 {
		t.Fatal("missing tls.crt")
	}
	if len(got.Data[corev1.TLSPrivateKeyKey]) == 0 {
		t.Fatal("missing tls.key")
	}
	if got.Type != corev1.SecretTypeTLS {
		t.Fatalf("expected tls secret type, got %s", got.Type)
	}

	var updated cmapi.Certificate
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "my", Namespace: "ns"}, &updated); err != nil {
		t.Fatalf("get cert: %v", err)
	}
	if len(updated.Status.Conditions) == 0 || updated.Status.Conditions[0].Status != cmmeta.ConditionTrue {
		t.Fatalf("expected Ready=True condition, got %#v", updated.Status.Conditions)
	}
}

// TestReconcileSkipsUnannotated ensures Certificates without our annotation
// are ignored — the predicate should drop them before the loop body runs.
func TestReconcileSkipsUnannotated(t *testing.T) {
	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{Name: "my", Namespace: "ns"},
		Spec: cmapi.CertificateSpec{
			SecretName: "my-tls",
			IssuerRef:  cmmeta.IssuerReference{Name: "prod", Group: "tencent.cert-manager.io", Kind: "TencentIssuer"},
		},
	}
	cli := ctrlfake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(cert).
		WithStatusSubresource(&cmapi.Certificate{}).
		Build()
	r := &IssuerReconciler{Client: cli, NewClient: nil}

	if _, err := r.Reconcile(context.Background(), newReq("my", "ns")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var s corev1.Secret
	err := cli.Get(context.Background(), types.NamespacedName{Name: "my-tls", Namespace: "ns"}, &s)
	if err == nil {
		t.Fatal("did not expect a Secret to be created for unannotated Certificate")
	}
}
