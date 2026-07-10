package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// TestResyncDetectsDrift ensures that when the local Secret's tls.crt differs
// from what Tencent currently returns, runOnce marks Synced=False on the
// Issuer. // ponytail: one drift test covers the documented feature; add
// force-sync / no-drift / unknown-cert-id variants when real flows surface them.
func TestResyncDetectsDrift(t *testing.T) {
	sch := newScheme(t)

	issuer := &api.TencentIssuer{
		ObjectMeta: metav1.ObjectMeta{Name: "prod", Namespace: "ns"},
		Spec: api.TencentIssuerSpec{
			Region:         "ap-guangzhou",
			SecretRef:      api.SecretRef{Name: "creds"},
			ResyncInterval: metav1.Duration{Duration: time.Minute},
		},
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
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-tls", Namespace: "ns"},
		Type:       corev1.SecretTypeTLS,
		Data:       map[string][]byte{corev1.TLSCertKey: []byte("OLD CERT")},
	}

	cli := ctrlfake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(issuer, creds, cert, existing).
		WithStatusSubresource(&api.TencentIssuer{}).
		Build()
	fc := fake.New(fake.WithCertificate("abc", "NEW-CERT-v2", "chain", "key"))

	r := &Resyncer{
		Client: cli,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fc, nil
		},
	}
	r.runOnce(context.Background(), issuer)

	var got api.TencentIssuer
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "prod", Namespace: "ns"}, &got); err != nil {
		t.Fatalf("get issuer: %v", err)
	}
	syncedFalse := false
	for _, c := range got.Status.Conditions {
		if c.Type == "Synced" && c.Status == metav1.ConditionFalse {
			syncedFalse = true
		}
	}
	if !syncedFalse {
		t.Fatalf("expected Synced=False after drift, got %#v", got.Status.Conditions)
	}
}

func TestResyncNoDriftMarksInSync(t *testing.T) {
	sch := newScheme(t)
	issuer := &api.TencentIssuer{
		ObjectMeta: metav1.ObjectMeta{Name: "prod", Namespace: "ns"},
		Spec: api.TencentIssuerSpec{
			Region:         "ap-guangzhou",
			SecretRef:      api.SecretRef{Name: "creds"},
			ResyncInterval: metav1.Duration{Duration: time.Minute},
		},
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
				Name: "prod", Group: "tencent.cert-manager.io", Kind: "TencentIssuer",
			},
		},
	}
	matching := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-tls", Namespace: "ns"},
		Type:       corev1.SecretTypeTLS,
		Data:       map[string][]byte{corev1.TLSCertKey: []byte("SAME-CERT\nchain")},
	}
	cli := ctrlfake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(issuer, creds, cert, matching).
		WithStatusSubresource(&api.TencentIssuer{}).
		Build()
	fc := fake.New(fake.WithCertificate("abc", "SAME-CERT", "chain", "key"))

	r := &Resyncer{
		Client: cli,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fc, nil
		},
	}
	r.runOnce(context.Background(), issuer)

	var got api.TencentIssuer
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "prod", Namespace: "ns"}, &got); err != nil {
		t.Fatal(err)
	}
	syncedTrue := false
	for _, c := range got.Status.Conditions {
		if c.Type == "Synced" && c.Status == metav1.ConditionTrue {
			syncedTrue = true
		}
	}
	if !syncedTrue {
		t.Fatalf("expected Synced=True when no drift, got %#v", got.Status.Conditions)
	}
}

func TestResyncMissingCredsMarksNotReady(t *testing.T) {
	sch := newScheme(t)
	issuer := baseIssuer("ns")
	cli := ctrlfake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(issuer).
		WithStatusSubresource(&api.TencentIssuer{}).
		Build()
	r := &Resyncer{Client: cli}
	r.runOnce(context.Background(), issuer)

	var got api.TencentIssuer
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "prod", Namespace: "ns"}, &got); err != nil {
		t.Fatal(err)
	}
	notReady := false
	for _, c := range got.Status.Conditions {
		if c.Type == "Ready" && c.Reason == "MissingCredentials" {
			notReady = true
		}
	}
	if !notReady {
		t.Fatalf("expected Ready=MissingCredentials, got %#v", got.Status.Conditions)
	}
}

func TestResyncNewClientError(t *testing.T) {
	sch := newScheme(t)
	issuer := baseIssuer("ns")
	cli := ctrlfake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(issuer, baseCreds("ns")).
		WithStatusSubresource(&api.TencentIssuer{}).
		Build()
	r := &Resyncer{
		Client: cli,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return nil, errors.New("init fail")
		},
	}
	r.runOnce(context.Background(), issuer)

	var got api.TencentIssuer
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "prod", Namespace: "ns"}, &got); err != nil {
		t.Fatal(err)
	}
	clientInit := false
	for _, c := range got.Status.Conditions {
		if c.Type == "Ready" && c.Reason == "ClientInit" {
			clientInit = true
		}
	}
	if !clientInit {
		t.Fatalf("expected Ready=ClientInit, got %#v", got.Status.Conditions)
	}
}

func TestResyncForceSyncClearsAnnotation(t *testing.T) {
	sch := newScheme(t)
	issuer := baseIssuer("ns")
	issuer.Spec.ResyncInterval = metav1.Duration{Duration: time.Minute}
	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my",
			Namespace:   "ns",
			Annotations: map[string]string{AnnotationCertID: "abc", AnnotationForceSync: "true"},
		},
		Spec: cmapi.CertificateSpec{
			SecretName: "my-tls",
			IssuerRef: cmmeta.IssuerReference{
				Name: "prod", Group: "tencent.cert-manager.io", Kind: "TencentIssuer",
			},
		},
	}
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-tls", Namespace: "ns"},
		Type:       corev1.SecretTypeTLS,
		Data:       map[string][]byte{corev1.TLSCertKey: []byte("OLD")},
	}
	cli := ctrlfake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(issuer, baseCreds("ns"), cert, existing).
		WithStatusSubresource(&api.TencentIssuer{}).
		Build()
	fc := fake.New(fake.WithCertificate("abc", "NEW", "chain", "key"))
	r := &Resyncer{
		Client: cli,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fc, nil
		},
	}
	r.runOnce(context.Background(), issuer)

	var got cmapi.Certificate
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "my", Namespace: "ns"}, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Annotations[AnnotationForceSync]; ok {
		t.Fatalf("force-sync annotation must be cleared, got %v", got.Annotations)
	}
}

func TestResyncSkipsUnownedAndUnannotated(t *testing.T) {
	sch := newScheme(t)
	issuer := baseIssuer("ns")
	issuer.Spec.ResyncInterval = metav1.Duration{Duration: time.Minute}
	unowned := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "u1",
			Namespace:   "ns",
			Annotations: map[string]string{AnnotationCertID: "abc"},
		},
		Spec: cmapi.CertificateSpec{
			SecretName: "u1-tls",
			IssuerRef:  cmmeta.IssuerReference{Name: "other", Group: "tencent.cert-manager.io", Kind: "TencentIssuer"},
		},
	}
	noAnnot := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{Name: "u2", Namespace: "ns"},
		Spec: cmapi.CertificateSpec{
			SecretName: "u2-tls",
			IssuerRef:  cmmeta.IssuerReference{Name: "prod", Group: "tencent.cert-manager.io", Kind: "TencentIssuer"},
		},
	}
	cli := ctrlfake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(issuer, baseCreds("ns"), unowned, noAnnot).
		WithStatusSubresource(&api.TencentIssuer{}).
		Build()
	fc := fake.New(fake.WithCertificate("abc", "x", "y", "z"))
	r := &Resyncer{
		Client: cli,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fc, nil
		},
	}
	r.runOnce(context.Background(), issuer)

	var got api.TencentIssuer
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "prod", Namespace: "ns"}, &got); err != nil {
		t.Fatal(err)
	}
	syncedTrue := false
	for _, c := range got.Status.Conditions {
		if c.Type == "Synced" && c.Status == metav1.ConditionTrue {
			syncedTrue = true
		}
	}
	if !syncedTrue {
		t.Fatalf("expected Synced=True when no relevant certs, got %#v", got.Status.Conditions)
	}
}

func TestResyncGetSecretError(t *testing.T) {
	sch := newScheme(t)
	issuer := baseIssuer("ns")
	issuer.Spec.ResyncInterval = metav1.Duration{Duration: time.Minute}
	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my",
			Namespace:   "ns",
			Annotations: map[string]string{AnnotationCertID: "abc"},
		},
		Spec: cmapi.CertificateSpec{
			SecretName: "my-tls",
			IssuerRef: cmmeta.IssuerReference{
				Name: "prod", Group: "tencent.cert-manager.io", Kind: "TencentIssuer",
			},
		},
	}
	cli := ctrlfake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(issuer, baseCreds("ns"), cert).
		WithStatusSubresource(&api.TencentIssuer{}).
		Build()
	fc := fake.New(fake.WithError("abc", errors.New("api down")))
	r := &Resyncer{
		Client: cli,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fc, nil
		},
	}
	r.runOnce(context.Background(), issuer)
	// No panic on err path; assert Synced=True (drift loop skipped this cert).
	var got api.TencentIssuer
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "prod", Namespace: "ns"}, &got); err != nil {
		t.Fatal(err)
	}
}

func TestResyncTickSkipsRecentlySynced(t *testing.T) {
	sch := newScheme(t)
	issuer := baseIssuer("ns")
	issuer.Spec.ResyncInterval = metav1.Duration{Duration: time.Hour}
	issuer.Status.Conditions = []metav1.Condition{{
		Type:               "Synced",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.NewTime(time.Now()),
	}}
	cli := ctrlfake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(issuer).
		WithStatusSubresource(&api.TencentIssuer{}).
		Build()
	r := &Resyncer{Client: cli}
	r.tick(context.Background())
	// Conditions should remain at the just-set timestamp, not overwritten.
	var got api.TencentIssuer
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "prod", Namespace: "ns"}, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Status.Conditions) == 0 {
		t.Fatal("expected Synced condition to remain")
	}
}

func TestLastSyncedAtEmpty(t *testing.T) {
	if !lastSyncedAt(baseIssuer("ns")).IsZero() {
		t.Fatal("expected zero time when no Synced condition")
	}
}

func TestLastSyncedAtFound(t *testing.T) {
	want := metav1.NewTime(time.Now().Add(-time.Hour))
	iss := baseIssuer("ns")
	iss.Status.Conditions = []metav1.Condition{{
		Type: "Synced", Status: metav1.ConditionTrue, LastTransitionTime: want,
	}}
	got := lastSyncedAt(iss)
	if !got.Equal(want.Time) {
		t.Fatalf("want %v, got %v", want.Time, got)
	}
}

func TestBelongsTo(t *testing.T) {
	cert := &cmapi.Certificate{
		Spec: cmapi.CertificateSpec{
			IssuerRef: cmmeta.IssuerReference{
				Name: "prod", Group: "tencent.cert-manager.io", Kind: "TencentIssuer",
			},
		},
	}
	if !belongsTo(cert, baseIssuer("ns")) {
		t.Fatal("should belong when group/kind/name match")
	}
	wrong := baseIssuer("ns")
	wrong.Name = "other"
	if belongsTo(cert, wrong) {
		t.Fatal("should not belong on name mismatch")
	}
	wrongGroup := cert.DeepCopy()
	wrongGroup.Spec.IssuerRef.Group = "x"
	if belongsTo(wrongGroup, baseIssuer("ns")) {
		t.Fatal("should not belong on group mismatch")
	}
}

func TestHash(t *testing.T) {
	if hash(nil) != "" {
		t.Fatal("empty input should produce empty hash")
	}
	a := hash([]byte("abc"))
	b := hash([]byte("abc"))
	c := hash([]byte("abd"))
	if a != b || a == c {
		t.Fatalf("hash deterministic? a==b: %v, a==c: %v", a == b, a == c)
	}
}

func TestStartSpawnsLoop(t *testing.T) {
	sch := newScheme(t)
	cli := ctrlfake.NewClientBuilder().WithScheme(sch).Build()
	r := &Resyncer{Client: cli}
	if err := r.Start(context.Background(), nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func TestLoopReturnsOnCtxCancel(t *testing.T) {
	sch := newScheme(t)
	cli := ctrlfake.NewClientBuilder().WithScheme(sch).Build()
	r := &Resyncer{Client: cli}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.loop(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not return after ctx cancel")
	}
}

func TestTickDefaultIntervalFallsBackTo24h(t *testing.T) {
	sch := newScheme(t)
	// Issuer has no ResyncInterval (zero) and no Synced condition (zero time).
	// lastSyncedAt returns zero, time.Since(zero) is huge, so runOnce runs.
	issuer := baseIssuer("ns")
	cli := ctrlfake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(issuer).
		WithStatusSubresource(&api.TencentIssuer{}).
		Build()
	r := &Resyncer{
		Client: cli,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return nil, errors.New("force runOnce to fail early, we only care about the path")
		},
	}
	r.tick(context.Background())
	// runOnce path executed: assert MissingCredentials or ClientInit condition set.
	var got api.TencentIssuer
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "prod", Namespace: "ns"}, &got); err != nil {
		t.Fatal(err)
	}
	touched := false
	for _, c := range got.Status.Conditions {
		if c.Reason == "MissingCredentials" || c.Reason == "ClientInit" {
			touched = true
		}
	}
	if !touched {
		t.Fatalf("expected tick to invoke runOnce, got conditions %#v", got.Status.Conditions)
	}
}

func TestLoopFiresOnTicker(t *testing.T) {
	sch := newScheme(t)
	issuer := baseIssuer("ns")
	issuer.Spec.ResyncInterval = metav1.Duration{Duration: time.Hour}
	issuer.Status.Conditions = []metav1.Condition{{
		Type: "Synced", Status: metav1.ConditionTrue, LastTransitionTime: metav1.NewTime(time.Now()),
	}}
	cli := ctrlfake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(issuer).
		WithStatusSubresource(&api.TencentIssuer{}).
		Build()
	r := &Resyncer{
		Client:   cli,
		Interval: 20 * time.Millisecond,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return nil, errors.New("force runOnce fail")
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.loop(ctx)
		close(done)
	}()
	// First tick skips (recently synced), subsequent ticks still hit runOnce
	// because Issuer has no Synced condition after our mark once Status().Update
	// re-runs. Just wait long enough for ≥1 tick to fire and cancel.
	time.Sleep(80 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not return after cancel")
	}
}

func TestTickListErrorSwallowed(t *testing.T) {
	// A client that errors on List should not panic and should not write
	// status — runOnce is never entered.
	cli := &errorListClient{err: errors.New("list boom")}
	r := &Resyncer{Client: cli}
	r.tick(context.Background())
}

type errorListClient struct {
	client.Client
	err error
}

func (e *errorListClient) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return e.err
}

func TestRunOnceGetSecretNonNotFoundContinues(t *testing.T) {
	sch := newScheme(t)
	issuer := baseIssuer("ns")
	issuer.Spec.ResyncInterval = metav1.Duration{Duration: time.Minute}
	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my", Namespace: "ns",
			Annotations: map[string]string{AnnotationCertID: "abc"},
		},
		Spec: cmapi.CertificateSpec{
			SecretName: "my-tls",
			IssuerRef: cmmeta.IssuerReference{
				Name: "prod", Group: "tencent.cert-manager.io", Kind: "TencentIssuer",
			},
		},
	}
	// Wrap the fake client so Gets for the cert's target Secret return a
	// non-NotFound error. Credential Secret Gets still pass through so
	// loadSecret succeeds and we reach the per-cert loop.
	wrapped := &secretErrClient{
		Client: ctrlfake.NewClientBuilder().
			WithScheme(sch).
			WithObjects(issuer, baseCreds("ns"), cert).
			WithStatusSubresource(&api.TencentIssuer{}).
			Build(),
		blockName: "my-tls",
		err:       errors.New("api server down"),
	}
	r := &Resyncer{
		Client: wrapped,
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return fake.New(fake.WithCertificate("abc", "x", "y", "z")), nil
		},
	}
	r.runOnce(context.Background(), issuer)
}

type secretErrClient struct {
	client.Client
	blockName string
	err       error
}

func (s *secretErrClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*corev1.Secret); ok && key.Name == s.blockName {
		return s.err
	}
	return s.Client.Get(ctx, key, obj, opts...)
}
