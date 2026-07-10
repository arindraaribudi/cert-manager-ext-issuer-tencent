package main

import (
	"flag"
	"fmt"
	"os"

	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/arindraaribudi/cert-manager-ext-issuer-tencent/internal/app"
)

// configureLogger installs the controller-runtime zap logger. Split out as a
// named function so tests can call it directly to cover the body.
func configureLogger() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
}

// Overridable for tests so main() can be exercised without a real kubeconfig
// or signal handler. // ponytail: package vars are the cheapest injection —
// if you grow more than a handful, switch to a struct.
var (
	getConfig     = ctrl.GetConfigOrDie
	signalHandler = ctrl.SetupSignalHandler
	setLogger     = configureLogger
)

// run is the testable body of main. Returns the process exit code.
func run(args []string) int {
	fs := flag.NewFlagSet("controller", flag.ContinueOnError)
	var (
		metricsAddr string
		healthAddr  string
		leaderElect bool
	)
	fs.StringVar(&metricsAddr, "metrics-addr", ":8080", "Metrics bind address.")
	fs.StringVar(&healthAddr, "health-addr", ":8081", "Health probe bind address.")
	fs.BoolVar(&leaderElect, "leader-elect", true, "Enable leader election.")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	setLogger()
	if err := app.Run(signalHandler(), getConfig(), app.Options{
		MetricsAddr: metricsAddr,
		HealthAddr:  healthAddr,
		LeaderElect: leaderElect,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// Main runs the controller with the current process args. Exported so tests
// can invoke it; main() is just a thin os.Exit wrapper.
func Main() int {
	return run(os.Args)
}

// exitFunc is os.Exit by default; tests override it to a no-op so the test
// process isn't terminated when Main() returns.
var exitFunc = os.Exit

func main() {
	exitFunc(Main())
}

// rest.Config is referenced indirectly through the getConfig type.
var _ = (*rest.Config)(nil)
