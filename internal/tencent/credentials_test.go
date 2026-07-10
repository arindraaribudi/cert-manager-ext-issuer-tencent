package tencent

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLoadStaticCredentialsMissingSecret(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	if _, err := LoadStaticCredentials(context.Background(), cli, "nope", "ns"); err == nil {
		t.Fatal("expected error for missing secret")
	}
}

func TestLoadStaticCredentialsEmptyKeys(t *testing.T) {
	sch := runtime.NewScheme()
	_ = corev1.AddToScheme(sch)
	cli := fake.NewClientBuilder().WithScheme(sch).WithObjects(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "ns"},
	}).Build()
	if _, err := LoadStaticCredentials(context.Background(), cli, "creds", "ns"); err == nil {
		t.Fatal("expected error for missing keys")
	}
}

func TestLoadStaticCredentialsOK(t *testing.T) {
	sch := runtime.NewScheme()
	_ = corev1.AddToScheme(sch)
	cli := fake.NewClientBuilder().WithScheme(sch).WithObjects(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "ns"},
		Data:       map[string][]byte{"secret-id": []byte("id"), "secret-key": []byte("key")},
	}).Build()
	got, err := LoadStaticCredentials(context.Background(), cli, "creds", "ns")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("credential must not be nil")
	}
}

func TestBuildCredentialProviderExhaustsChain(t *testing.T) {
	// No TKE env, no STS env, no CVM metadata — chain fails.
	// Set sentinel env vars to clean state and unset via t.Setenv.
	t.Setenv("TENCENTCLOUD_SECRET_ID", "")
	t.Setenv("TENCENTCLOUD_SECRET_KEY", "")
	if _, err := BuildCredentialProvider(context.Background()); err == nil {
		t.Fatal("expected error when no provider is usable")
	}
}

func TestBuildCredentialProviderPrefersEnvOverMetadata(t *testing.T) {
	t.Setenv("TENCENTCLOUD_SECRET_ID", "id")
	t.Setenv("TENCENTCLOUD_SECRET_KEY", "key")
	got, err := BuildCredentialProvider(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sid, skey, _ := got.GetCredential()
	if sid != "id" || skey != "key" {
		t.Fatalf("env provider should win, got sid=%q skey=%q", sid, skey)
	}
}
