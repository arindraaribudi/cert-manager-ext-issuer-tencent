// Package controller watches cert-manager Certificate CRs that carry a
// tencent.cert-manager.io/certificate-id annotation and mirrors the underlying
// Tencent SSL certificate (chain + private key) into the target Secret.
// Replaces the issuer-lib Sign callback because PEMBundle has no slot for the
// private key. // ponytail: drop-in for issuer-lib Sign, custom because issuer-lib drops the key
package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/tencentcloud/tencentcloud-sdk-go-intl-en/tencentcloud/common"

	api "github.com/arindraaribudi/cert-manager-ext-issuer-tencent/api/v1alpha1"
	"github.com/arindraaribudi/cert-manager-ext-issuer-tencent/internal/conditions"
	"github.com/arindraaribudi/cert-manager-ext-issuer-tencent/internal/tencent"
)

const finalizerName = "tencent.cert-manager.io/finalizer"

type IssuerReconciler struct {
	client.Client
	NewClient func(creds common.CredentialIface, region, endpoint string) (tencent.Client, error)
}

// hasCertIDAnnotation reports whether the object carries the
// tencent.cert-manager.io/certificate-id annotation — the signal that this
// controller should mirror the upstream Tencent cert into the target Secret.
func hasCertIDAnnotation(obj client.Object) bool {
	_, ok := obj.GetAnnotations()[AnnotationCertID]
	return ok
}

func (r *IssuerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return SetupIssuerWithName(mgr, r, "tencentcertificate")
}

// SetupIssuerWithName is like SetupWithManager but lets callers override the
// controller name — used by tests to avoid the global name uniqueness check.
func SetupIssuerWithName(mgr ctrl.Manager, r *IssuerReconciler, name string) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cmapi.Certificate{}, builder.WithPredicates(predicate.NewPredicateFuncs(hasCertIDAnnotation))).
		Named(name).
		Owns(&corev1.Secret{}).
		Watches(&api.TencentClusterIssuer{}, handler.EnqueueRequestsFromMapFunc(r.enqueueCertsForClusterIssuer)).
		Complete(r)
}

// enqueueCertsForClusterIssuer maps a cluster-scoped issuer to every Certificate
// whose issuerRef points at it. No field-selector on issuerRef.name without a
// custom indexer, so we List all Certificates and filter in code. // ponytail:
// List + filter is fine at v1alpha scale; add a field indexer when cert count
// grows enough that full Lists dominate reconcile latency.
func (r *IssuerReconciler) enqueueCertsForClusterIssuer(ctx context.Context, obj client.Object) []ctrl.Request {
	ci, ok := obj.(*api.TencentClusterIssuer)
	if !ok {
		return nil
	}
	var certs cmapi.CertificateList
	if err := r.List(ctx, &certs); err != nil {
		return nil
	}
	var out []ctrl.Request
	for _, c := range certs.Items {
		if c.Spec.IssuerRef.Group == "tencent.cert-manager.io" &&
			c.Spec.IssuerRef.Kind == "TencentClusterIssuer" &&
			c.Spec.IssuerRef.Name == ci.Name {
			out = append(out, ctrl.Request{NamespacedName: types.NamespacedName{Name: c.Name, Namespace: c.Namespace}})
		}
	}
	return out
}

// loadCredentials picks the auth source: static k8s Secret when secretRef.Name
// is set, otherwise the SDK credential chain (TKE pod-identity → STS env → CVM
// instance metadata role).
func (r *IssuerReconciler) loadCredentials(ctx context.Context, spec api.TencentIssuerSpec, fallbackNS string) (common.CredentialIface, error) {
	if spec.SecretRef.Name != "" {
		ns := spec.SecretRef.Namespace
		if ns == "" {
			ns = fallbackNS
		}
		return tencent.LoadStaticCredentials(ctx, r.Client, spec.SecretRef.Name, ns)
	}
	return tencent.BuildCredentialProvider(ctx)
}

func (r *IssuerReconciler) resolveIssuer(ctx context.Context, cert *cmapi.Certificate) (*api.TencentIssuerSpec, string, error) {
	if cert.Spec.IssuerRef.Group != "tencent.cert-manager.io" {
		return nil, "", fmt.Errorf("unexpected issuer group %q", cert.Spec.IssuerRef.Group)
	}
	switch cert.Spec.IssuerRef.Kind {
	case "TencentClusterIssuer":
		var issuer api.TencentClusterIssuer
		if err := r.Get(ctx, types.NamespacedName{Name: cert.Spec.IssuerRef.Name}, &issuer); err != nil {
			return nil, "", fmt.Errorf("get cluster issuer: %w", err)
		}
		ns := issuer.Spec.SecretRef.Namespace
		if ns == "" {
			ns = cert.Namespace
		}
		return &issuer.Spec, ns, nil
	case "TencentIssuer":
		var issuer api.TencentIssuer
		if err := r.Get(ctx, types.NamespacedName{Name: cert.Spec.IssuerRef.Name, Namespace: cert.Namespace}, &issuer); err != nil {
			return nil, "", fmt.Errorf("get issuer: %w", err)
		}
		ns := issuer.Spec.SecretRef.Namespace
		if ns == "" {
			ns = cert.Namespace
		}
		return &issuer.Spec, ns, nil
	default:
		return nil, "", fmt.Errorf("unexpected issuer kind %q", cert.Spec.IssuerRef.Kind)
	}
}

func (r *IssuerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cert cmapi.Certificate
	if err := r.Get(ctx, req.NamespacedName, &cert); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	certID, ok := CertIDFromCertificate(&cert)
	if !ok {
		return ctrl.Result{}, nil
	}

	if !cert.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&cert, finalizerName) {
			controllerutil.RemoveFinalizer(&cert, finalizerName)
			if err := r.Update(ctx, &cert); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}
	if !controllerutil.ContainsFinalizer(&cert, finalizerName) {
		controllerutil.AddFinalizer(&cert, finalizerName)
		if err := r.Update(ctx, &cert); err != nil {
			return ctrl.Result{}, err
		}
	}

	issuerSpec, secretNS, err := r.resolveIssuer(ctx, &cert)
	if err != nil {
		conditions.SetCertificateCondition(&cert, cmmeta.ConditionFalse, "IssuerNotFound", err.Error())
		_ = r.Status().Update(ctx, &cert)
		return ctrl.Result{}, err
	}

	creds, err := r.loadCredentials(ctx, *issuerSpec, secretNS)
	if err != nil {
		conditions.SetCertificateCondition(&cert, cmmeta.ConditionFalse, "MissingCredentials", err.Error())
		_ = r.Status().Update(ctx, &cert)
		return ctrl.Result{}, err
	}

	cli, err := r.NewClient(creds, issuerSpec.Region, issuerSpec.Endpoint)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("new tencent client: %w", err)
	}

	if _, err := cli.DescribeCertificate(ctx, certID); err != nil {
		conditions.SetCertificateCondition(&cert, cmmeta.ConditionFalse, "DescribeFailed", err.Error())
		_ = r.Status().Update(ctx, &cert)
		return ctrl.Result{}, err
	}

	dl, err := cli.DownloadCertificate(ctx, certID)
	if err != nil {
		conditions.SetCertificateCondition(&cert, cmmeta.ConditionFalse, "DownloadFailed", err.Error())
		_ = r.Status().Update(ctx, &cert)
		return ctrl.Result{}, err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cert.Spec.SecretName,
			Namespace: cert.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(&cert, schema.GroupVersionKind{
					Group: "cert-manager.io", Version: "v1", Kind: "Certificate",
				}),
			},
			Annotations: map[string]string{
				"cert-manager.io/issuer-name":  cert.Spec.IssuerRef.Name,
				"cert-manager.io/issuer-kind":  cert.Spec.IssuerRef.Kind,
				"cert-manager.io/issuer-group": cert.Spec.IssuerRef.Group,
			},
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       dl.Certificates,
			corev1.TLSPrivateKeyKey: dl.PrivateKey,
		},
	}

	existing := &corev1.Secret{}
	getErr := r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, existing)
	switch {
	case apierrors.IsNotFound(getErr):
		if err := r.Create(ctx, secret); err != nil {
			return ctrl.Result{}, err
		}
	case getErr != nil:
		return ctrl.Result{}, getErr
	default:
		existing.Data = secret.Data
		secret.OwnerReferences = mergeOwnerRefs(existing.OwnerReferences, secret.OwnerReferences)
		existing.OwnerReferences = secret.OwnerReferences
		if err := r.Update(ctx, existing); err != nil {
			return ctrl.Result{}, err
		}
	}

	conditions.SetCertificateCondition(&cert, cmmeta.ConditionTrue, "Synced", "certificate and key synced from Tencent")
	if err := r.Status().Update(ctx, &cert); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// mergeOwnerRefs returns the union of existing and ours, deduplicated by UID.
// our refs are listed first so the Certificate controller ref stays primary on the Secret.
func mergeOwnerRefs(existing, ours []metav1.OwnerReference) []metav1.OwnerReference {
	seen := map[types.UID]bool{}
	out := []metav1.OwnerReference{}
	for _, r := range ours {
		if !seen[r.UID] {
			seen[r.UID] = true
			out = append(out, r)
		}
	}
	for _, r := range existing {
		if !seen[r.UID] {
			seen[r.UID] = true
			out = append(out, r)
		}
	}
	return out
}
