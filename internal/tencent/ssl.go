package tencent

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/tencentcloud/tencentcloud-sdk-go-intl-en/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go-intl-en/tencentcloud/common/profile"
	ssl "github.com/tencentcloud/tencentcloud-sdk-go-intl-en/tencentcloud/ssl/v20191205"
)

type SSLClient struct {
	api      sslAPI
	endpoint string
}

// sslAPI is the subset of *ssl.Client used by SSLClient. Defined as an interface
// so tests can substitute a fake without touching the Tencent SDK.
type sslAPI interface {
	DescribeCertificatesWithContext(ctx context.Context, req *ssl.DescribeCertificatesRequest) (*ssl.DescribeCertificatesResponse, error)
	DownloadCertificateWithContext(ctx context.Context, req *ssl.DownloadCertificateRequest) (*ssl.DownloadCertificateResponse, error)
}

// NewSSLClient builds an SSL API client from any SDK credential (static,
// TKE OIDC, env, CVM role, ...). The credential handles its own refresh.
func NewSSLClient(creds common.CredentialIface, region, endpoint string) (*SSLClient, error) {
	if creds == nil {
		return nil, fmt.Errorf("tencent: nil credential")
	}
	if endpoint == "" {
		endpoint = "ssl.tencentcloudapi.com"
	}
	prof := profile.NewClientProfile()
	prof.HttpProfile.Endpoint = endpoint
	cli, err := ssl.NewClient(creds, region, prof)
	if err != nil {
		return nil, fmt.Errorf("tencent: new ssl client: %w", err)
	}
	return newSSLClientWithAPI(cli, endpoint), nil
}

func newSSLClientWithAPI(api sslAPI, endpoint string) *SSLClient {
	if endpoint == "" {
		endpoint = "ssl.tencentcloudapi.com"
	}
	return &SSLClient{
		api:      api,
		endpoint: endpoint,
	}
}

func (c *SSLClient) DescribeCertificate(ctx context.Context, id string) (*Certificate, error) {
	req := ssl.NewDescribeCertificatesRequest()
	req.SearchKey = common.StringPtr(id)
	resp, err := c.api.DescribeCertificatesWithContext(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("describe: %w", err)
	}
	if resp == nil || resp.Response == nil || len(resp.Response.Certificates) == 0 {
		return nil, fmt.Errorf("certificate %q not found", id)
	}
	var c0 *ssl.Certificates
	for _, x := range resp.Response.Certificates {
		if x != nil && x.CertificateId != nil && *x.CertificateId == id {
			c0 = x
			break
		}
	}
	if c0 == nil {
		c0 = resp.Response.Certificates[0]
	}
	out := &Certificate{ID: id, Name: strDeref(c0.Alias)}
	if c0.Status != nil {
		out.Status = strconv.FormatUint(*c0.Status, 10)
	}
	if c0.CertBeginTime != nil {
		if t, err := time.Parse(time.RFC3339, *c0.CertBeginTime); err == nil {
			out.NotBefore = t
		}
	}
	if c0.CertEndTime != nil {
		if t, err := time.Parse(time.RFC3339, *c0.CertEndTime); err == nil {
			out.NotAfter = t
		}
	}
	return out, nil
}

// DownloadCertificate fetches the issued cert + chain + key. The intl-en
// SSL API returns the ZIP as Base64 in the response body (no separate URL
// fetch step like the original SDK).
func (c *SSLClient) DownloadCertificate(ctx context.Context, id string) (*DownloadedCert, error) {
	req := ssl.NewDownloadCertificateRequest()
	req.CertificateId = common.StringPtr(id)
	resp, err := c.api.DownloadCertificateWithContext(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	if resp == nil || resp.Response == nil || resp.Response.Content == nil {
		return nil, fmt.Errorf("download content empty for %q", id)
	}
	body, err := base64.StdEncoding.DecodeString(*resp.Response.Content)
	if err != nil {
		return nil, fmt.Errorf("decode content: %w", err)
	}
	ex, err := ExtractFromZIP(body)
	if err != nil {
		return nil, err
	}
	chain, err := AssembleTLS(ex.Leaf, ex.Chain)
	if err != nil {
		return nil, err
	}
	return &DownloadedCert{
		Certificates: chain,
		Chain:        ex.Chain,
		PrivateKey:   ex.PrivateKey,
	}, nil
}

func strDeref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

var _ Client = (*SSLClient)(nil)
