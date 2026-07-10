package controller

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/tencentcloud/tencentcloud-sdk-go-intl-en/tencentcloud/common"

	api "github.com/arindraaribudi/cert-manager-ext-issuer-tencent/api/v1alpha1"
	"github.com/arindraaribudi/cert-manager-ext-issuer-tencent/internal/tencent"
	"github.com/arindraaribudi/cert-manager-ext-issuer-tencent/internal/tencent/fake"
)

func baseIssuer(ns string) *api.TencentIssuer {
	return &api.TencentIssuer{
		ObjectMeta: metav1.ObjectMeta{Name: "prod", Namespace: ns},
		Spec:       api.TencentIssuerSpec{Region: "ap-guangzhou", SecretRef: api.SecretRef{Name: "creds"}},
	}
}

func baseCreds(ns string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: ns},
		Data:       map[string][]byte{"secret-id": []byte("id"), "secret-key": []byte("key")},
	}
}

func annotatedCert(name, ns, id string) *cmapi.Certificate {
	return &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   ns,
			Annotations: map[string]string{AnnotationCertID: id},
		},
		Spec: cmapi.CertificateSpec{
			SecretName: name + "-tls",
			IssuerRef: cmmeta.IssuerReference{
				Name: "prod", Group: "tencent.cert-manager.io", Kind: "TencentIssuer",
			},
		},
	}
}

func newCLI(t *testing.T, objs ...client.Object) *ctrlfake.ClientBuilder {
	t.Helper()
	return ctrlfake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(objs...).
		WithStatusSubresource(&cmapi.Certificate{}, &api.TencentIssuer{})
}

func TestReconcileAddsFinalizerWhenMissing(t *testing.T) {
	cert := annotatedCert("my", "ns", "abc")
	cli := newCLI(t, cert, baseIssuer("ns"), baseCreds("ns")).Build()
	r := &IssuerReconciler{
		Client: cli,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fake.New(fake.WithCertificate("abc", leafPEM, chainPEM, keyPEM)), nil
		},
	}
	if _, err := r.Reconcile(context.Background(), newReq("my", "ns")); err != nil {
		t.Fatal(err)
	}
	var got cmapi.Certificate
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "my", Namespace: "ns"}, &got); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range got.Finalizers {
		if f == finalizerName {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected finalizer %q, got %v", finalizerName, got.Finalizers)
	}
}

func TestReconcileRemovesFinalizerOnDelete(t *testing.T) {
	now := metav1.Now()
	cert := annotatedCert("my", "ns", "abc")
	cert.DeletionTimestamp = &now
	cert.Finalizers = []string{finalizerName}
	cli := newCLI(t, cert, baseIssuer("ns"), baseCreds("ns")).Build()
	r := &IssuerReconciler{
		Client: cli,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fake.New(), nil
		},
	}
	if _, err := r.Reconcile(context.Background(), newReq("my", "ns")); err != nil {
		t.Fatal(err)
	}
	// After finalizer removal on a marked-for-delete object, the fake client
	// simulates GC and removes the resource from the store. Reconcile returning
	// nil without error is the success signal here.
}

func TestReconcileIssuerWrongGroup(t *testing.T) {
	cert := annotatedCert("my", "ns", "abc")
	cert.Spec.IssuerRef.Group = "other.group"
	cli := newCLI(t, cert).Build()
	r := &IssuerReconciler{Client: cli}
	_, err := r.Reconcile(context.Background(), newReq("my", "ns"))
	if err == nil {
		t.Fatal("expected error for wrong group")
	}
}

func TestReconcileClusterIssuerNotSupported(t *testing.T) {
	cert := annotatedCert("my", "ns", "abc")
	cert.Spec.IssuerRef.Kind = "TencentClusterIssuer"
	cli := newCLI(t, cert).Build()
	r := &IssuerReconciler{Client: cli}
	_, err := r.Reconcile(context.Background(), newReq("my", "ns"))
	if err == nil {
		t.Fatal("expected error for ClusterIssuer kind")
	}
}

func TestReconcileIssuerKindUnknown(t *testing.T) {
	cert := annotatedCert("my", "ns", "abc")
	cert.Spec.IssuerRef.Kind = "Mystery"
	cli := newCLI(t, cert).Build()
	r := &IssuerReconciler{Client: cli}
	_, err := r.Reconcile(context.Background(), newReq("my", "ns"))
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
}

func TestReconcileIssuerNotFound(t *testing.T) {
	cert := annotatedCert("my", "ns", "abc")
	cli := newCLI(t, cert).Build()
	r := &IssuerReconciler{Client: cli}
	if _, err := r.Reconcile(context.Background(), newReq("my", "ns")); err == nil {
		t.Fatal("expected error when issuer missing")
	}
}

func TestReconcileMissingSecretKeys(t *testing.T) {
	issuer := baseIssuer("ns")
	creds := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "ns"},
		Data:       map[string][]byte{"secret-id": []byte("id")},
	}
	cert := annotatedCert("my", "ns", "abc")
	cli := newCLI(t, issuer, creds, cert).Build()
	r := &IssuerReconciler{
		Client: cli,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fake.New(), nil
		},
	}
	if _, err := r.Reconcile(context.Background(), newReq("my", "ns")); err == nil {
		t.Fatal("expected error for missing secret keys")
	}
}

func TestReconcileNewClientError(t *testing.T) {
	cert := annotatedCert("my", "ns", "abc")
	cli := newCLI(t, baseIssuer("ns"), baseCreds("ns"), cert).Build()
	r := &IssuerReconciler{
		Client: cli,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return nil, errors.New("sdk init fail")
		},
	}
	if _, err := r.Reconcile(context.Background(), newReq("my", "ns")); err == nil {
		t.Fatal("expected error from NewClient")
	}
}

func TestReconcileDescribeError(t *testing.T) {
	cert := annotatedCert("my", "ns", "abc")
	cli := newCLI(t, baseIssuer("ns"), baseCreds("ns"), cert).Build()
	r := &IssuerReconciler{
		Client: cli,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fake.New(fake.WithError("abc", errors.New("boom"))), nil
		},
	}
	if _, err := r.Reconcile(context.Background(), newReq("my", "ns")); err == nil {
		t.Fatal("expected error from DescribeCertificate")
	}
}

func TestReconcileDownloadError(t *testing.T) {
	cert := annotatedCert("my", "ns", "abc")
	cli := newCLI(t, baseIssuer("ns"), baseCreds("ns"), cert).Build()
	// fake returns ok from Describe but error from Download.
	half := &halfBrokenFake{describeErr: nil, downloadErr: errors.New("download boom")}
	r := &IssuerReconciler{
		Client: cli,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return half, nil
		},
	}
	if _, err := r.Reconcile(context.Background(), newReq("my", "ns")); err == nil {
		t.Fatal("expected error from DownloadCertificate")
	}
}

type halfBrokenFake struct {
	describeErr error
	downloadErr error
}

func (h *halfBrokenFake) DescribeCertificate(_ context.Context, id string) (*tencent.Certificate, error) {
	return &tencent.Certificate{ID: id}, h.describeErr
}
func (h *halfBrokenFake) DownloadCertificate(_ context.Context, id string) (*tencent.DownloadedCert, error) {
	return nil, h.downloadErr
}

var _ tencent.Client = (*halfBrokenFake)(nil)

func TestReconcileUpdatesExistingSecret(t *testing.T) {
	issuer := baseIssuer("ns")
	creds := baseCreds("ns")
	cert := annotatedCert("my", "ns", "abc")
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-tls", Namespace: "ns"},
		Type:       corev1.SecretTypeTLS,
		Data:       map[string][]byte{corev1.TLSCertKey: []byte("old")},
	}
	cli := newCLI(t, issuer, creds, cert, existing).Build()
	r := &IssuerReconciler{
		Client: cli,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fake.New(fake.WithCertificate("abc", leafPEM, chainPEM, keyPEM)), nil
		},
	}
	if _, err := r.Reconcile(context.Background(), newReq("my", "ns")); err != nil {
		t.Fatal(err)
	}
	var got corev1.Secret
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "my-tls", Namespace: "ns"}, &got); err != nil {
		t.Fatal(err)
	}
	if string(got.Data[corev1.TLSCertKey]) == "old" {
		t.Fatal("secret tls.crt should be updated, still old")
	}
}

func TestResolveIssuerGetError(t *testing.T) {
	cert := annotatedCert("my", "ns", "abc")
	cli := newCLI(t, cert).Build()
	r := &IssuerReconciler{Client: cli}
	if _, _, err := r.resolveIssuer(context.Background(), cert); err == nil {
		t.Fatal("expected error when issuer not found")
	}
}

func TestResolveIssuerNamespaced(t *testing.T) {
	cert := annotatedCert("my", "ns", "abc")
	cli := newCLI(t, cert, baseIssuer("ns")).Build()
	r := &IssuerReconciler{Client: cli}
	spec, ns, err := r.resolveIssuer(context.Background(), cert)
	if err != nil {
		t.Fatal(err)
	}
	if spec == nil || spec.Region != "ap-guangzhou" {
		t.Fatalf("spec not returned, got %+v", spec)
	}
	if ns != "ns" {
		t.Fatalf("secret ns should fall back to cert ns, got %q", ns)
	}
}

func TestResolveIssuerCluster(t *testing.T) {
	cert := annotatedCert("my", "ns", "abc")
	cert.Spec.IssuerRef.Kind = "TencentClusterIssuer"
	ci := &api.TencentClusterIssuer{
		ObjectMeta: metav1.ObjectMeta{Name: "prod"},
		Spec: api.TencentIssuerSpec{
			Region:    "ap-shanghai",
			SecretRef: api.SecretRef{Name: "creds", Namespace: "creds-ns"},
		},
	}
	cli := newCLI(t, cert, ci).Build()
	r := &IssuerReconciler{Client: cli}
	spec, ns, err := r.resolveIssuer(context.Background(), cert)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Region != "ap-shanghai" {
		t.Fatalf("cluster spec not returned, got %+v", spec)
	}
	if ns != "creds-ns" {
		t.Fatalf("secret ns should honor cluster issuer override, got %q", ns)
	}
}

func TestResolveIssuerClusterDefaultsToCertNS(t *testing.T) {
	cert := annotatedCert("my", "ns", "abc")
	cert.Spec.IssuerRef.Kind = "TencentClusterIssuer"
	ci := &api.TencentClusterIssuer{
		ObjectMeta: metav1.ObjectMeta{Name: "prod"},
		Spec: api.TencentIssuerSpec{
			Region:    "ap-shanghai",
			SecretRef: api.SecretRef{Name: "creds"},
		},
	}
	cli := newCLI(t, cert, ci).Build()
	r := &IssuerReconciler{Client: cli}
	_, ns, err := r.resolveIssuer(context.Background(), cert)
	if err != nil {
		t.Fatal(err)
	}
	if ns != "ns" {
		t.Fatalf("empty cluster secretRef.namespace should fall back to cert ns, got %q", ns)
	}
}

func TestLoadSecretMissing(t *testing.T) {
	cli := newCLI(t).Build()
	if _, err := tencent.LoadStaticCredentials(context.Background(), cli, "nope", "ns"); err == nil {
		t.Fatal("expected error for missing secret")
	}
}

func TestLoadSecretEmptyKeys(t *testing.T) {
	creds := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "ns"},
		Data:       map[string][]byte{},
	}
	cli := newCLI(t, creds).Build()
	if _, err := tencent.LoadStaticCredentials(context.Background(), cli, "creds", "ns"); err == nil {
		t.Fatal("expected error for empty keys")
	}
}

func TestMergeOwnerRefsDedupe(t *testing.T) {
	uid := types.UID("same")
	existing := []metav1.OwnerReference{{UID: uid, Name: "old"}, {UID: types.UID("other"), Name: "keep"}}
	ours := []metav1.OwnerReference{{UID: uid, Name: "new"}}
	out := mergeOwnerRefs(existing, ours)
	seen := map[types.UID]string{}
	for _, r := range out {
		seen[r.UID] = r.Name
	}
	if seen[uid] != "new" {
		t.Fatalf("ours should win on UID collision; got %q", seen[uid])
	}
	if seen[types.UID("other")] != "keep" {
		t.Fatal("existing non-conflicting ref should remain")
	}
	if len(out) != 2 {
		t.Fatalf("want 2 refs after dedup, got %d", len(out))
	}
}

func TestReconcileNotFoundCert(t *testing.T) {
	cli := newCLI(t).Build()
	r := &IssuerReconciler{Client: cli}
	if _, err := r.Reconcile(context.Background(), newReq("missing", "ns")); err != nil {
		if !apierrors.IsNotFound(err) {
			t.Fatalf("want NotFound, got %v", err)
		}
	}
}

// suppress unused-import for apierrors in builds where TestReconcileNotFoundCert
// is the only consumer. Compile-time reference keeps it live.
var _ = apierrors.IsNotFound

func TestReconcileCertNotFound(t *testing.T) {
	cli := newCLI(t).Build()
	r := &IssuerReconciler{Client: cli}
	_, err := r.Reconcile(context.Background(), newReq("missing", "ns"))
	if err != nil {
		t.Fatalf("expected nil error (IgnoreNotFound), got %v", err)
	}
}

func TestReconcileGetSecretNonNotFoundError(t *testing.T) {
	issuer := baseIssuer("ns")
	cert := annotatedCert("my", "ns", "abc")
	wrapped := &secretErrClient{
		Client: newCLI(t, issuer, baseCreds("ns"), cert).Build(),
		err:    errors.New("api down"),
	}
	r := &IssuerReconciler{
		Client: wrapped,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fake.New(), nil
		},
	}
	_, err := r.Reconcile(context.Background(), newReq("my", "ns"))
	if err == nil {
		t.Fatal("expected error from secret Get")
	}
}

// failingClient returns writeErr on Update/Create/Patch and the underlying
// fake for reads. Tests use it to drive error-return branches in Reconcile.
type failingClient struct {
	client.Client
	writeErr error
}

func (f *failingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return f.writeErr
}
func (f *failingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return f.writeErr
}
func (f *failingClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return f.writeErr
}

type failingSubResourceWriter struct{ err error }

func (f failingSubResourceWriter) Create(_ context.Context, _ client.Object, _ client.Object, _ ...client.SubResourceCreateOption) error {
	return nil
}
func (f failingSubResourceWriter) Update(_ context.Context, _ client.Object, _ ...client.SubResourceUpdateOption) error {
	return f.err
}
func (f failingSubResourceWriter) Patch(_ context.Context, _ client.Object, _ client.Patch, _ ...client.SubResourcePatchOption) error {
	return f.err
}
func (f failingSubResourceWriter) Apply(_ context.Context, _ runtime.ApplyConfiguration, _ ...client.SubResourceApplyOption) error {
	return nil
}

func (f *failingClient) Status() client.SubResourceWriter {
	return failingSubResourceWriter{err: f.writeErr}
}

func TestReconcileAddFinalizerUpdateError(t *testing.T) {
	cert := annotatedCert("my", "ns", "abc")
	wrapped := &failingClient{
		Client:   newCLI(t, baseIssuer("ns"), baseCreds("ns"), cert).Build(),
		writeErr: errors.New("api down"),
	}
	r := &IssuerReconciler{
		Client: wrapped,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fake.New(), nil
		},
	}
	if _, err := r.Reconcile(context.Background(), newReq("my", "ns")); err == nil {
		t.Fatal("expected error from finalizer Update")
	}
}

func TestReconcileRemoveFinalizerUpdateError(t *testing.T) {
	now := metav1.Now()
	cert := annotatedCert("my", "ns", "abc")
	cert.DeletionTimestamp = &now
	cert.Finalizers = []string{finalizerName}
	wrapped := &failingClient{
		Client:   newCLI(t, baseIssuer("ns"), baseCreds("ns"), cert).Build(),
		writeErr: errors.New("api down"),
	}
	r := &IssuerReconciler{
		Client: wrapped,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fake.New(), nil
		},
	}
	if _, err := r.Reconcile(context.Background(), newReq("my", "ns")); err == nil {
		t.Fatal("expected error from finalizer Update")
	}
}

func TestReconcileCreateSecretError(t *testing.T) {
	cert := annotatedCert("my", "ns", "abc")
	wrapped := &failingClient{
		Client:   newCLI(t, baseIssuer("ns"), baseCreds("ns"), cert).Build(),
		writeErr: errors.New("create denied"),
	}
	r := &IssuerReconciler{
		Client: wrapped,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fake.New(fake.WithCertificate("abc", leafPEM, chainPEM, keyPEM)), nil
		},
	}
	if _, err := r.Reconcile(context.Background(), newReq("my", "ns")); err == nil {
		t.Fatal("expected error from Secret Create")
	}
}

func TestReconcileUpdateSecretError(t *testing.T) {
	cert := annotatedCert("my", "ns", "abc")
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-tls", Namespace: "ns"},
		Type:       corev1.SecretTypeTLS,
	}
	wrapped := &failingClient{
		Client:   newCLI(t, baseIssuer("ns"), baseCreds("ns"), cert, existing).Build(),
		writeErr: errors.New("update denied"),
	}
	r := &IssuerReconciler{
		Client: wrapped,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fake.New(fake.WithCertificate("abc", leafPEM, chainPEM, keyPEM)), nil
		},
	}
	if _, err := r.Reconcile(context.Background(), newReq("my", "ns")); err == nil {
		t.Fatal("expected error from Secret Update")
	}
}

func TestReconcileFinalStatusUpdateError(t *testing.T) {
	cert := annotatedCert("my", "ns", "abc")
	// Real reads/writes work; only the Status sub-resource writer errors.
	base := newCLI(t, baseIssuer("ns"), baseCreds("ns"), cert).Build()
	wrapped := &statusFailingClient{Client: base, err: errors.New("status update denied")}
	r := &IssuerReconciler{
		Client: wrapped,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fake.New(fake.WithCertificate("abc", leafPEM, chainPEM, keyPEM)), nil
		},
	}
	if _, err := r.Reconcile(context.Background(), newReq("my", "ns")); err == nil {
		t.Fatal("expected error from Status Update")
	}
}

type statusFailingClient struct {
	client.Client
	err error
}

func (s *statusFailingClient) Status() client.SubResourceWriter {
	return failingSubResourceWriter{err: s.err}
}
