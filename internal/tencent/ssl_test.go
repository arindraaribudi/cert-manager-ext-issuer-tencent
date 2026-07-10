package tencent

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/tencentcloud/tencentcloud-sdk-go-intl-en/tencentcloud/common"
	ssl "github.com/tencentcloud/tencentcloud-sdk-go-intl-en/tencentcloud/ssl/v20191205"
)

// fakeSSLAPI substitutes *ssl.Client for unit tests. Set fields per-test.
type fakeSSLAPI struct {
	describeResp *ssl.DescribeCertificatesResponse
	describeErr  error
	downloadResp *ssl.DownloadCertificateResponse
	downloadErr  error
}

func (f *fakeSSLAPI) DescribeCertificatesWithContext(_ context.Context, _ *ssl.DescribeCertificatesRequest) (*ssl.DescribeCertificatesResponse, error) {
	return f.describeResp, f.describeErr
}

func (f *fakeSSLAPI) DownloadCertificateWithContext(_ context.Context, _ *ssl.DownloadCertificateRequest) (*ssl.DownloadCertificateResponse, error) {
	return f.downloadResp, f.downloadErr
}

// ponytail: SSLClient's SDK calls hit the real Tencent API; only the
// DownloadCertificate decode/extract path and NewSSLClient validation can be
// driven from a unit test. SDK-backed paths stay uncovered until integration
// tests with a recorded fixture — add then.

func TestNewSSLClientRejectsNilCreds(t *testing.T) {
	if _, err := NewSSLClient(nil, "ap-guangzhou", ""); err == nil {
		t.Fatal("expected error for nil creds")
	}
}

func TestNewSSLClientSuccess(t *testing.T) {
	c, err := NewSSLClient(common.NewCredential("id", "key"), "ap-guangzhou", "ssl.example.com")
	if err != nil {
		t.Fatalf("expected success: %v", err)
	}
	if c.endpoint != "ssl.example.com" {
		t.Fatalf("endpoint: %q", c.endpoint)
	}
	if c.api == nil {
		t.Fatal("api must be populated")
	}
}

func TestNewSSLClientDefaultEndpoint(t *testing.T) {
	c, err := NewSSLClient(common.NewCredential("id", "key"), "ap-guangzhou", "")
	if err != nil {
		t.Fatalf("expected success: %v", err)
	}
	if c.endpoint != "ssl.tencentcloudapi.com" {
		t.Fatalf("endpoint: %q", c.endpoint)
	}
}

func TestStrDeref(t *testing.T) {
	if got := strDeref(nil); got != "" {
		t.Fatalf("nil: want empty, got %q", got)
	}
	s := "x"
	if got := strDeref(&s); got != "x" {
		t.Fatalf("non-nil: want x, got %q", got)
	}
}

func TestNewSSLClientWithAPIDefaultEndpoint(t *testing.T) {
	c := newSSLClientWithAPI(&fakeSSLAPI{}, "")
	if c.endpoint != "ssl.tencentcloudapi.com" {
		t.Fatalf("want default endpoint, got %q", c.endpoint)
	}
	c2 := newSSLClientWithAPI(&fakeSSLAPI{}, "custom.example.com")
	if c2.endpoint != "custom.example.com" {
		t.Fatalf("want custom endpoint, got %q", c2.endpoint)
	}
}

func TestDescribeCertificateParseAllFields(t *testing.T) {
	status := uint64(1)
	api := &fakeSSLAPI{
		describeResp: &ssl.DescribeCertificatesResponse{
			Response: &ssl.DescribeCertificatesResponseParams{
				Certificates: []*ssl.Certificates{
					{
						CertificateId: stringPtr("abc"),
						Alias:         stringPtr("alias-x"),
						Status:        &status,
						CertBeginTime: stringPtr("2025-01-01T00:00:00Z"),
						CertEndTime:   stringPtr("2026-01-01T00:00:00Z"),
					},
				},
			},
		},
	}
	c := newSSLClientWithAPI(api, "")
	cert, err := c.DescribeCertificate(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if cert.ID != "abc" || cert.Name != "alias-x" || cert.Status != "1" {
		t.Fatalf("unexpected: %#v", cert)
	}
	if cert.NotBefore.IsZero() || cert.NotAfter.IsZero() {
		t.Fatal("times not parsed")
	}
}

func TestDescribeCertificateParseBadTimesIgnored(t *testing.T) {
	api := &fakeSSLAPI{
		describeResp: &ssl.DescribeCertificatesResponse{
			Response: &ssl.DescribeCertificatesResponseParams{
				Certificates: []*ssl.Certificates{{
					CertificateId: stringPtr("abc"),
					CertBeginTime: stringPtr("not-a-time"),
					CertEndTime:   stringPtr("also-not"),
				}},
			},
		},
	}
	c := newSSLClientWithAPI(api, "")
	cert, err := c.DescribeCertificate(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if !cert.NotBefore.IsZero() || !cert.NotAfter.IsZero() {
		t.Fatal("bad RFC3339 must not populate times")
	}
}

func TestDescribeCertificateError(t *testing.T) {
	c := newSSLClientWithAPI(&fakeSSLAPI{describeErr: errors.New("boom")}, "")
	if _, err := c.DescribeCertificate(context.Background(), "abc"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDescribeCertificateEmptyResponse(t *testing.T) {
	c := newSSLClientWithAPI(&fakeSSLAPI{describeResp: &ssl.DescribeCertificatesResponse{}}, "")
	if _, err := c.DescribeCertificate(context.Background(), "abc"); err == nil {
		t.Fatal("expected error for empty response")
	}
	c2 := newSSLClientWithAPI(&fakeSSLAPI{describeResp: &ssl.DescribeCertificatesResponse{Response: &ssl.DescribeCertificatesResponseParams{}}}, "")
	if _, err := c2.DescribeCertificate(context.Background(), "abc"); err == nil {
		t.Fatal("expected error for empty certificates")
	}
}

func TestDescribeCertificateFallbackToFirst(t *testing.T) {
	api := &fakeSSLAPI{
		describeResp: &ssl.DescribeCertificatesResponse{
			Response: &ssl.DescribeCertificatesResponseParams{
				Certificates: []*ssl.Certificates{
					{CertificateId: stringPtr("other")},
					{CertificateId: stringPtr("another")},
				},
			},
		},
	}
	c := newSSLClientWithAPI(api, "")
	cert, err := c.DescribeCertificate(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if cert.ID != "abc" {
		t.Fatalf("want id 'abc', got %q", cert.ID)
	}
}

func TestDescribeCertificateNilAlias(t *testing.T) {
	api := &fakeSSLAPI{
		describeResp: &ssl.DescribeCertificatesResponse{
			Response: &ssl.DescribeCertificatesResponseParams{
				Certificates: []*ssl.Certificates{{CertificateId: stringPtr("abc"), Alias: nil}},
			},
		},
	}
	c := newSSLClientWithAPI(api, "")
	cert, err := c.DescribeCertificate(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if cert.Name != "" {
		t.Fatalf("nil alias must produce empty name, got %q", cert.Name)
	}
}

func TestDownloadCertificateError(t *testing.T) {
	c := newSSLClientWithAPI(&fakeSSLAPI{downloadErr: errors.New("boom")}, "")
	if _, err := c.DownloadCertificate(context.Background(), "abc"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDownloadCertificateEmptyContent(t *testing.T) {
	c := newSSLClientWithAPI(&fakeSSLAPI{
		downloadResp: &ssl.DownloadCertificateResponse{},
	}, "")
	if _, err := c.DownloadCertificate(context.Background(), "abc"); err == nil {
		t.Fatal("expected error for empty content")
	}
	c2 := newSSLClientWithAPI(&fakeSSLAPI{
		downloadResp: &ssl.DownloadCertificateResponse{
			Response: &ssl.DownloadCertificateResponseParams{},
		},
	}, "")
	if _, err := c2.DownloadCertificate(context.Background(), "abc"); err == nil {
		t.Fatal("expected error when content nil")
	}
}

func TestDownloadCertificateInvalidBase64(t *testing.T) {
	api := &fakeSSLAPI{
		downloadResp: &ssl.DownloadCertificateResponse{
			Response: &ssl.DownloadCertificateResponseParams{
				Content: stringPtr("!!!not base64!!!"),
			},
		},
	}
	c := newSSLClientWithAPI(api, "")
	if _, err := c.DownloadCertificate(context.Background(), "abc"); err == nil {
		t.Fatal("expected base64 decode error")
	}
}

func TestDownloadCertificateInvalidZip(t *testing.T) {
	api := &fakeSSLAPI{
		downloadResp: &ssl.DownloadCertificateResponse{
			Response: &ssl.DownloadCertificateResponseParams{
				Content: stringPtr(base64.StdEncoding.EncodeToString([]byte("not a zip"))),
			},
		},
	}
	c := newSSLClientWithAPI(api, "")
	if _, err := c.DownloadCertificate(context.Background(), "abc"); err == nil {
		t.Fatal("expected error for invalid zip")
	}
}

func TestDownloadCertificateSuccess(t *testing.T) {
	zipBody := buildZIP(t, map[string]string{
		"leaf.crt":  leafPEM,
		"chain.crt": chainPEM,
		"leaf.key":  keyPEM,
	})
	api := &fakeSSLAPI{
		downloadResp: &ssl.DownloadCertificateResponse{
			Response: &ssl.DownloadCertificateResponseParams{
				Content: stringPtr(base64.StdEncoding.EncodeToString(zipBody)),
			},
		},
	}
	c := newSSLClientWithAPI(api, "")
	dl, err := c.DownloadCertificate(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(dl.Certificates) == 0 || len(dl.PrivateKey) == 0 {
		t.Fatalf("missing parts: %#v", dl)
	}
}

func TestDownloadCertificateLeafMissingPEMEnd(t *testing.T) {
	// Force AssembleTLS error: leaf has no END marker AND chain is non-empty
	// (so the end-marker check actually runs). Use ordered zip so leaf is
	// picked first regardless of map iteration order.
	brokenLeaf := "-----BEGIN CERTIFICATE-----\nbroken\n"
	zipBody := buildZIPOrdered(t, []zipEntry{
		{Name: "leaf.crt", Body: brokenLeaf},
		{Name: "chain.crt", Body: chainPEM},
		{Name: "leaf.key", Body: keyPEM},
	})
	api := &fakeSSLAPI{
		downloadResp: &ssl.DownloadCertificateResponse{
			Response: &ssl.DownloadCertificateResponseParams{
				Content: stringPtr(base64.StdEncoding.EncodeToString(zipBody)),
			},
		},
	}
	c := newSSLClientWithAPI(api, "")
	if _, err := c.DownloadCertificate(context.Background(), "abc"); err == nil {
		t.Fatal("expected error from AssembleTLS missing PEM end")
	}
}

func stringPtr(s string) *string { return &s }
