package controller

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/tencentcloud/tencentcloud-sdk-go-intl-en/tencentcloud/common"

	api "github.com/arindraaribudi/cert-manager-ext-issuer-tencent/api/v1alpha1"
	"github.com/arindraaribudi/cert-manager-ext-issuer-tencent/internal/tencent"
)

// TestSetupWithManagerEnvtest exercises the real ctrl.Manager wiring via
// envtest. Skipped when KUBEBUILDER_ASSETS is unset. // ponytail: one envtest,
// not many — covers the only path that actually touches a Manager.
func TestSetupWithManagerEnvtest(t *testing.T) {
	assets := os.Getenv("KUBEBUILDER_ASSETS")
	if assets == "" {
		t.Skip("KUBEBUILDER_ASSETS not set; run via `make test`")
	}
	if _, err := os.Stat(filepath.Join(assets, "kube-apiserver")); err != nil {
		t.Skipf("envtest assets missing: %v", err)
	}

	te := &envtest.Environment{BinaryAssetsDirectory: assets}
	cfg, err := te.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	defer func() {
		_ = te.Stop()
	}()

	s := scheme.Scheme
	if err := api.AddToScheme(s); err != nil {
		t.Fatalf("api scheme: %v", err)
	}
	if err := cmapi.AddToScheme(s); err != nil {
		t.Fatalf("cmapi scheme: %v", err)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: s})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	r := &IssuerReconciler{
		Client: mgr.GetClient(),
		NewClient: func(_ common.CredentialIface, _, _ string) (tencent.Client, error) {
			return nil, nil
		},
	}
	if err := r.SetupWithManager(mgr); err != nil {
		t.Fatalf("SetupWithManager: %v", err)
	}
}
