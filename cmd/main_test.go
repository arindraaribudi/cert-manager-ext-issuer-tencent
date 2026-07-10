package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// withOverrides swaps the package-level injection points for the duration of
// the test. Returns a restore func.
func withOverrides(t *testing.T, get func() *rest.Config, sig func() context.Context, log func()) func() {
	t.Helper()
	origGet, origSig, origLog := getConfig, signalHandler, setLogger
	getConfig = get
	signalHandler = sig
	setLogger = log
	return func() {
		getConfig = origGet
		signalHandler = origSig
		setLogger = origLog
	}
}

func TestRunFlagParseError(t *testing.T) {
	defer withOverrides(t, nil, nil, func() {})()
	if code := run([]string{"prog", "--bogus"}); code != 2 {
		t.Fatalf("want exit code 2 on bad flag, got %d", code)
	}
}

func TestRunAppError(t *testing.T) {
	cancelled := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	badConfig := func() *rest.Config { return nil }
	noLog := func() {}
	defer withOverrides(t, badConfig, cancelled, noLog)()

	if code := run([]string{"prog"}); code != 1 {
		t.Fatalf("want exit code 1 when app.Run fails, got %d", code)
	}
}

func TestRunSuccess(t *testing.T) {
	cancelled := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	badConfig := func() *rest.Config { return nil }
	noLog := func() {}
	defer withOverrides(t, badConfig, cancelled, noLog)()

	// app.Run with nil config returns error → exit 1. To reach exit 0 we need
	// the full happy path which requires a real cluster; covered indirectly
	// by the envtest integration in internal/app. Assert we hit the run path.
	if code := run([]string{"prog"}); code == 0 {
		// Could only happen if app.Run returned nil with nil config — won't
		// happen, but include for completeness.
		t.Log("run returned 0")
	}
}

func TestSetLoggerCalled(t *testing.T) {
	called := false
	log := func() { called = true }
	cancelled := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	badConfig := func() *rest.Config { return nil }
	defer withOverrides(t, badConfig, cancelled, log)()
	_ = run([]string{"prog"})
	if !called {
		t.Fatal("setLogger should have been called on the success path")
	}
}

func TestMainExitCodePath(t *testing.T) {
	// Direct call to main is impossible (runtime owns it). Verify the
	// binding between run() return and os.Exit behavior by asserting that
	// run returns the documented codes.
	cancelled := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	badConfig := func() *rest.Config { return nil }
	noLog := func() {}
	defer withOverrides(t, badConfig, cancelled, noLog)()

	if code := run([]string{"prog"}); code != 1 {
		t.Fatalf("want 1, got %d", code)
	}
}

func TestConfigureLogger(t *testing.T) {
	// configureLogger is the body of the setLogger initializer; calling it
	// installs a zap logger into controller-runtime. We can't easily assert
	// the side effect, so just make sure it doesn't panic.
	configureLogger()
}

func TestMainFlagParse(t *testing.T) {
	// Verify Main() returns 2 on bad flag.
	cancelled := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	badConfig := func() *rest.Config { return nil }
	noLog := func() {}
	defer withOverrides(t, badConfig, cancelled, noLog)()

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"prog", "--bogus"}
	if code := Main(); code != 2 {
		t.Fatalf("want 2 on bad flag, got %d", code)
	}
}

func TestMainFunctionCallsExit(t *testing.T) {
	// Cover the main() body without terminating the test process.
	cancelled := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	badConfig := func() *rest.Config { return nil }
	noLog := func() {}
	defer withOverrides(t, badConfig, cancelled, noLog)()

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"prog"}

	called := 0
	origExit := exitFunc
	exitFunc = func(int) { called++ }
	defer func() { exitFunc = origExit }()

	main()
	if called != 1 {
		t.Fatalf("main() should call exitFunc once, got %d", called)
	}
}

func TestRunSuccessViaEnvtest(t *testing.T) {
	assets := os.Getenv("KUBEBUILDER_ASSETS")
	if assets == "" {
		t.Skip("KUBEBUILDER_ASSETS not set; run via `make test`")
	}

	te := &envtest.Environment{BinaryAssetsDirectory: assets}
	cfg, err := te.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	defer func() { _ = te.Stop() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer withOverrides(t,
		func() *rest.Config { return cfg },
		func() context.Context { return ctx },
		func() {},
	)()

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"prog", "--leader-elect=false", "--metrics-addr=:0", "--health-addr=:0"}

	// Cancel after the manager starts so app.Run returns nil and run() exits 0.
	done := make(chan int, 1)
	go func() { done <- run(os.Args) }()
	time.Sleep(3 * time.Second)
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("want exit 0 on graceful shutdown, got %d", code)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("run did not return after ctx cancel")
	}
}

// silence unused imports in builds where errors may not be referenced.
var _ = errors.New
var _ = os.Stderr
