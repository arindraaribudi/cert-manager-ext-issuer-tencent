package tencent_test

import (
	"context"
	"testing"
	"time"

	"github.com/arindraaribudi/cert-manager-ext-issuer-tencent/internal/tencent"
)

type fakeClient struct{ got struct{ id string } }

func (f *fakeClient) DescribeCertificate(ctx context.Context, id string) (*tencent.Certificate, error) {
	f.got.id = id
	return &tencent.Certificate{ID: id, Status: "Issued", NotBefore: time.Now(), NotAfter: time.Now().Add(90 * 24 * time.Hour)}, nil
}
func (f *fakeClient) DownloadCertificate(ctx context.Context, id string) (*tencent.DownloadedCert, error) {
	return &tencent.DownloadedCert{Certificates: []byte("leaf"), Chain: []byte("chain"), PrivateKey: []byte("key")}, nil
}

var _ tencent.Client = (*fakeClient)(nil)

func TestClientInterface(t *testing.T) {
	var c tencent.Client = &fakeClient{}
	cert, err := c.DescribeCertificate(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if cert.ID != "abc" {
		t.Fatalf("want abc, got %s", cert.ID)
	}
	dl, err := c.DownloadCertificate(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.PrivateKey) == 0 {
		t.Fatal("empty key")
	}
}
