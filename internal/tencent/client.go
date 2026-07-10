package tencent

import "context"

type Client interface {
	DescribeCertificate(ctx context.Context, id string) (*Certificate, error)
	DownloadCertificate(ctx context.Context, id string) (*DownloadedCert, error)
}
