// Package app wires the controller-runtime manager, the IssuerReconciler,
// and the Resyncer. main.go stays a thin flag parser; everything testable
// lives here. // ponytail: split out so envtest can drive Run without
// needing a real kubeconfig or invoking os.Exit.
package app

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/tencentcloud/tencentcloud-sdk-go-intl-en/tencentcloud/common"

	api "github.com/arindraaribudi/cert-manager-ext-issuer-tencent/api/v1alpha1"
	ctrlpkg "github.com/arindraaribudi/cert-manager-ext-issuer-tencent/internal/controller"
	"github.com/arindraaribudi/cert-manager-ext-issuer-tencent/internal/tencent"
)

// Scheme is the runtime.Scheme preloaded with our API types + cert-manager.
// Exposed so tests can construct managers against the same scheme.
var Scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(api.AddToScheme(Scheme))
	utilruntime.Must(cmapi.AddToScheme(Scheme))
	utilruntime.Must(corev1.AddToScheme(Scheme))
}

// defaultControllerName is the controller name used in production. Tests
// override it via the test-only SetupControllersWithName to avoid the global
// name uniqueness check across sibling envtest cases.
const defaultControllerName = "tencentcertificate"

// NewClientFunc builds a tencent.Client from credentials. Tests inject a fake.
type NewClientFunc func(creds common.CredentialIface, region, endpoint string) (tencent.Client, error)

// Options configures the manager and its controllers.
type Options struct {
	MetricsAddr string
	HealthAddr  string
	LeaderElect bool
	NewClient   NewClientFunc
}

// NewManager builds a controller-runtime manager wired with metrics, health
// probes, and leader election. Returns the manager ready for controller setup.
func NewManager(cfg *rest.Config, opts Options) (ctrl.Manager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("create manager: nil rest config")
	}
	return ctrl.NewManager(cfg, manager.Options{
		Scheme:                 Scheme,
		Metrics:                metricsserver.Options{BindAddress: opts.MetricsAddr},
		HealthProbeBindAddress: opts.HealthAddr,
		LeaderElection:         opts.LeaderElect,
		LeaderElectionID:       "cert-manager-ext-issuer-tencent",
	})
}

// SetupControllers registers the IssuerReconciler with the manager.
func SetupControllers(mgr ctrl.Manager, newClient NewClientFunc) error {
	return SetupControllersWithName(mgr, newClient, defaultControllerName)
}

// SetupControllersWithName is the test-friendly variant that lets callers
// override the controller name (controller-runtime requires globally unique
// names, which makes sibling envtest cases conflict otherwise).
func SetupControllersWithName(mgr ctrl.Manager, newClient NewClientFunc, name string) error {
	if err := ctrlpkg.SetupIssuerWithName(mgr, &ctrlpkg.IssuerReconciler{
		Client:    mgr.GetClient(),
		NewClient: newClient,
	}, name); err != nil {
		return fmt.Errorf("setup issuer controller: %w", err)
	}
	return nil
}

// StartResyncer launches the periodic drift-detection loop. Always returns
// nil today (Resyncer.Start spawns a goroutine and never errors) — kept as
// an error-returning signature for future expansion.
func StartResyncer(ctx context.Context, mgr ctrl.Manager, newClient NewClientFunc) error {
	_ = (&ctrlpkg.Resyncer{
		Client:    mgr.GetClient(),
		NewClient: newClient,
	}).Start(ctx, mgr)
	return nil
}

// newSSLClientAdapter wraps tencent.NewSSLClient so it satisfies
// NewClientFunc (the concrete return type differs).
func newSSLClientAdapter(creds common.CredentialIface, region, endpoint string) (tencent.Client, error) {
	return tencent.NewSSLClient(creds, region, endpoint)
}

// Run wires everything together and blocks until ctx is canceled or the
// manager exits. Returns nil on graceful shutdown (ctx canceled); returns
// non-nil for setup failures or unexpected manager errors.
func Run(ctx context.Context, cfg *rest.Config, opts Options) error {
	return RunWithControllerName(ctx, cfg, opts, defaultControllerName)
}

// RunWithControllerName lets callers (mostly tests) override the controller
// name to avoid controller-runtime's global uniqueness check.
func RunWithControllerName(ctx context.Context, cfg *rest.Config, opts Options, controllerName string) error {
	if opts.NewClient == nil {
		opts.NewClient = newSSLClientAdapter
	}
	mgr, err := NewManager(cfg, opts)
	if err != nil {
		return err
	}
	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return fmt.Errorf("add healthz check: %w", err)
	}
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		return fmt.Errorf("add readyz check: %w", err)
	}
	if err := SetupControllersWithName(mgr, opts.NewClient, controllerName); err != nil {
		return err
	}
	_ = StartResyncer(ctx, mgr, opts.NewClient)
	// Run blocks until ctx is canceled or the manager exits. Treat context
	// cancellation as a graceful shutdown (no error).
	startErr := mgr.Start(ctx)
	if startErr != nil && !errors.Is(startErr, context.Canceled) {
		return startErr
	}
	return nil
}
