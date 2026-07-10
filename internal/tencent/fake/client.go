package fake

import (
	"context"
	"fmt"
	"time"

	"github.com/arindraaribudi/cert-manager-ext-issuer-tencent/internal/tencent"
)

type entry struct {
	cert tencent.Certificate
	dl   tencent.DownloadedCert
}

type Client struct {
	entries  map[string]*entry
	failWith map[string]error
}

type Option func(*Client)

func WithCertificate(id, leaf, chain, key string) Option {
	return func(c *Client) {
		c.entries[id] = &entry{
			cert: tencent.Certificate{ID: id, Status: "Issued", NotBefore: time.Now(), NotAfter: time.Now().Add(90 * 24 * time.Hour)},
			dl: tencent.DownloadedCert{
				Certificates: []byte(leaf + "\n" + chain),
				Chain:        []byte(chain),
				PrivateKey:   []byte(key),
			},
		}
	}
}

func WithError(id string, err error) Option {
	return func(c *Client) {
		c.failWith[id] = err
	}
}

func New(opts ...Option) *Client {
	c := &Client{entries: map[string]*entry{}, failWith: map[string]error{}}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Client) DescribeCertificate(ctx context.Context, id string) (*tencent.Certificate, error) {
	if err, ok := c.failWith[id]; ok {
		return nil, err
	}
	e, ok := c.entries[id]
	if !ok {
		return nil, fmt.Errorf("fake: cert %q not found", id)
	}
	cp := e.cert
	return &cp, nil
}

func (c *Client) DownloadCertificate(ctx context.Context, id string) (*tencent.DownloadedCert, error) {
	if err, ok := c.failWith[id]; ok {
		return nil, err
	}
	e, ok := c.entries[id]
	if !ok {
		return nil, fmt.Errorf("fake: cert %q not found", id)
	}
	cp := e.dl
	return &cp, nil
}

var _ tencent.Client = (*Client)(nil)
