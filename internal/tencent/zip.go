package tencent

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
)

type Extracted struct {
	Leaf       []byte
	Chain      []byte
	PrivateKey []byte
}

func ExtractFromZIP(data []byte) (*Extracted, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	out := &Extracted{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.Name, err)
		}
		switch {
		case bytes.Contains(body, []byte("PRIVATE KEY")) || bytes.Contains(body, []byte("RSA PRIVATE KEY")):
			out.PrivateKey = body
		case bytes.Contains(body, []byte("BEGIN CERTIFICATE")):
			if out.Leaf == nil {
				out.Leaf = body
			} else if out.Chain == nil {
				out.Chain = body
			} else {
				if len(out.Chain) > 0 && len(body) > len(out.Chain) {
					out.Chain = body
				}
			}
		}
	}
	if out.Leaf == nil {
		return nil, fmt.Errorf("zip missing certificate")
	}
	if out.PrivateKey == nil {
		return nil, fmt.Errorf("zip missing private key")
	}
	return out, nil
}

func AssembleTLS(leaf, chain []byte) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write(leaf)
	if len(chain) > 0 {
		if !bytes.HasSuffix(bytes.TrimSpace(leaf), []byte("-----END CERTIFICATE-----")) {
			return nil, fmt.Errorf("leaf missing PEM end marker")
		}
		buf.WriteByte('\n')
		buf.Write(chain)
	}
	return buf.Bytes(), nil
}
