package fake

import (
	"context"
	"errors"
	"testing"
)

func TestFakeClient(t *testing.T) {
	fc := New(WithCertificate("abc", "leafPEM", "chainPEM", "keyPEM"))
	cert, err := fc.DescribeCertificate(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if cert.ID != "abc" {
		t.Fatalf("want abc got %s", cert.ID)
	}
	dl, err := fc.DownloadCertificate(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if string(dl.PrivateKey) != "keyPEM" {
		t.Fatal("key mismatch")
	}
}

func TestFakeClientUnknownID(t *testing.T) {
	fc := New()
	if _, err := fc.DescribeCertificate(context.Background(), "missing"); err == nil {
		t.Fatal("expected error for unknown id")
	}
	if _, err := fc.DownloadCertificate(context.Background(), "missing"); err == nil {
		t.Fatal("expected error for unknown id")
	}
}

func TestFakeClientWithError(t *testing.T) {
	want := errors.New("boom")
	fc := New(WithError("abc", want))
	if _, err := fc.DescribeCertificate(context.Background(), "abc"); !errors.Is(err, want) {
		t.Fatalf("describe: want %v, got %v", want, err)
	}
	if _, err := fc.DownloadCertificate(context.Background(), "abc"); !errors.Is(err, want) {
		t.Fatalf("download: want %v, got %v", want, err)
	}
}

func TestFakeClientEmptyConstructor(t *testing.T) {
	fc := New()
	if fc.entries == nil || fc.failWith == nil {
		t.Fatal("constructor must init maps")
	}
}
