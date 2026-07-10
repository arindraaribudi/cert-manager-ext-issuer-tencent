// Resync ticker — the controller doesn't embed cert-manager's issuer-lib, so
// the only thing that re-mirrors a Tencent cert into the Secret is the
// Certificate reconciler. Without a trigger (annotation, cert-manager renewal
// timer, controller restart), a renewed upstream cert goes unnoticed. The
// ticker steps through every namespaced TencentIssuer, hashes what Tencent has
// now against what the Secret holds, and either clears the force-sync
// annotation (re-triggering reconcile) or just surfaces drift via the
// Issuer's Synced condition. // ponytail: kept narrow — only TencentIssuer,
// no ClusterIssuer; interval default 24h; drift signal is best-effort.
package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/tencentcloud/tencentcloud-sdk-go-intl-en/tencentcloud/common"

	api "github.com/arindraaribudi/cert-manager-ext-issuer-tencent/api/v1alpha1"
	"github.com/arindraaribudi/cert-manager-ext-issuer-tencent/internal/conditions"
	"github.com/arindraaribudi/cert-manager-ext-issuer-tencent/internal/tencent"
)

const (
	issuerGroup = "tencent.cert-manager.io"
	issuerKind  = "TencentIssuer"
)

type Resyncer struct {
	client.Client
	NewClient func(creds common.CredentialIface, region, endpoint string) (tencent.Client, error)

	// Interval controls how often the resync loop ticks. Zero means 1 minute.
	// Exposed for tests so the loop can be driven without minute-long sleeps.
	Interval time.Duration
}

// Start is wired into controller-runtime manager.Start(ctx, ...).
// The manager's ctx cancellation tears down the goroutine.
func (r *Resyncer) Start(ctx context.Context, mgr ctrl.Manager) error {
	go r.loop(ctx)
	return nil
}

func (r *Resyncer) loop(ctx context.Context) {
	interval := r.Interval
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

func (r *Resyncer) tick(ctx context.Context) {
	var issuers api.TencentIssuerList
	if err := r.List(ctx, &issuers); err != nil {
		return
	}
	for i := range issuers.Items {
		iss := &issuers.Items[i]
		interval := iss.Spec.ResyncInterval.Duration
		if interval <= 0 {
			interval = 24 * time.Hour
		}
		if time.Since(lastSyncedAt(iss)) < interval {
			continue
		}
		r.runOnce(ctx, iss)
	}
}

// lastSyncedAt returns the last Synced condition transition. A zero time
// means "never", which time.Since immediately exceeds the interval so the
// first tick always runs.
func lastSyncedAt(iss *api.TencentIssuer) time.Time {
	for _, c := range iss.Status.Conditions {
		if c.Type == "Synced" {
			return c.LastTransitionTime.Time
		}
	}
	return time.Time{}
}

// runOnce processes a single issuer — exposed so tests can drive it without
// spinning a real ticker. // ponytail: one entry point per issuer is enough;
// per-cert parallelism doesn't buy much at single-namespace scale.
func (r *Resyncer) runOnce(ctx context.Context, issuer *api.TencentIssuer) {
	var creds common.CredentialIface
	var err error
	if issuer.Spec.SecretRef.Name != "" {
		creds, err = tencent.LoadStaticCredentials(ctx, r.Client, issuer.Spec.SecretRef.Name, issuer.Namespace)
	} else {
		creds, err = tencent.BuildCredentialProvider(ctx)
	}
	if err != nil {
		conditions.SetReady(issuer, metav1.ConditionFalse, "MissingCredentials", err.Error())
		if uerr := r.Status().Update(ctx, issuer); uerr != nil {
			return
		}
		return
	}
	cli, err := r.NewClient(creds, issuer.Spec.Region, issuer.Spec.Endpoint)
	if err != nil {
		conditions.SetReady(issuer, metav1.ConditionFalse, "ClientInit", err.Error())
		_ = r.Status().Update(ctx, issuer)
		return
	}

	var certs cmapi.CertificateList
	_ = r.List(ctx, &certs, client.InNamespace(issuer.Namespace))

	synced := true
	for i := range certs.Items {
		cert := &certs.Items[i]
		if !belongsTo(cert, issuer) {
			continue
		}
		id, ok := CertIDFromCertificate(cert)
		if !ok {
			continue
		}
		force := IsForceSyncSet(cert)

		var secret corev1.Secret
		secretKey := types.NamespacedName{Name: cert.Spec.SecretName, Namespace: cert.Namespace}
		haveSecret := false
		if err := r.Get(ctx, secretKey, &secret); err == nil {
			haveSecret = true
		} else if !apierrors.IsNotFound(err) {
			continue
		}

		if _, err := cli.DescribeCertificate(ctx, id); err != nil {
			continue
		}
		dl, err := cli.DownloadCertificate(ctx, id)
		if err != nil {
			continue
		}

		remoteHash := hash(dl.Certificates)
		localHash := hash(secret.Data[corev1.TLSCertKey])
		if haveSecret && remoteHash == localHash && !force {
			continue
		}

		synced = false
		if force {
			patch := client.MergeFrom(cert.DeepCopy())
			delete(cert.Annotations, AnnotationForceSync)
			if err := r.Patch(ctx, cert, patch); err != nil {
				continue
			}
		}
	}

	if !synced {
		conditions.SetSynced(issuer, metav1.ConditionFalse, "RemoteChanged",
			fmt.Sprintf("drift detected at %s", time.Now().UTC().Format(time.RFC3339)))
	} else {
		conditions.SetSynced(issuer, metav1.ConditionTrue, "InSync", "all certificates in sync")
	}
	if err := r.Status().Update(ctx, issuer); err != nil {
		return
	}
}

func belongsTo(cert *cmapi.Certificate, issuer *api.TencentIssuer) bool {
	return cert.Spec.IssuerRef.Name == issuer.Name &&
		cert.Spec.IssuerRef.Group == issuerGroup &&
		cert.Spec.IssuerRef.Kind == issuerKind
}

func hash(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
