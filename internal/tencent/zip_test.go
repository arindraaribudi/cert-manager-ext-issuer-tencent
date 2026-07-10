package tencent

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

const (
	leafPEM  = "-----BEGIN CERTIFICATE-----\nMIIB...leaf\n-----END CERTIFICATE-----\n"
	chainPEM = "-----BEGIN CERTIFICATE-----\nMIIB...chain\n-----END CERTIFICATE-----\n"
	keyPEM   = "-----BEGIN PRIVATE KEY-----\nMIIE...key\n-----END PRIVATE KEY-----\n"
)

func buildZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type zipEntry struct {
	Name string
	Body string
}

// buildZIPOrdered writes entries in slice order — required when tests depend on
// which PEM ExtractFromZIP classifies as leaf vs chain (it picks first match).
func buildZIPOrdered(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.Body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractFromZIP(t *testing.T) {
	z := buildZIPOrdered(t, []zipEntry{
		{Name: "example.com.crt", Body: leafPEM},
		{Name: "example.com_ca.crt", Body: chainPEM},
		{Name: "example.com.key", Body: keyPEM},
	})
	out, err := ExtractFromZIP(z)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Leaf, []byte("leaf")) {
		t.Fatal("missing leaf")
	}
	if !bytes.Contains(out.Chain, []byte("chain")) {
		t.Fatal("missing chain")
	}
	if !bytes.Contains(out.PrivateKey, []byte("PRIVATE KEY")) {
		t.Fatal("missing key")
	}
}

func TestAssembleTLS(t *testing.T) {
	tls, err := AssembleTLS([]byte(leafPEM), []byte(chainPEM))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(tls, []byte("leaf")) || !bytes.Contains(tls, []byte("chain")) {
		t.Fatal("assembled TLS missing parts")
	}
}

func TestAssembleTLSNoChain(t *testing.T) {
	out, err := AssembleTLS([]byte(leafPEM), nil)
	if err != nil {
		t.Fatalf("no chain: %v", err)
	}
	if !bytes.Contains(out, []byte("leaf")) {
		t.Fatal("missing leaf")
	}
}

func TestAssembleTLSMissingPEMEnd(t *testing.T) {
	bad := []byte("-----BEGIN CERTIFICATE-----\nnot closed\n")
	if _, err := AssembleTLS(bad, []byte(chainPEM)); err == nil {
		t.Fatal("expected error for missing PEM end marker")
	}
}

func TestExtractFromZIPInvalidData(t *testing.T) {
	if _, err := ExtractFromZIP([]byte("not a zip")); err == nil {
		t.Fatal("expected error on non-zip data")
	}
}

func TestExtractFromZIPMissingCert(t *testing.T) {
	z := buildZIP(t, map[string]string{"a.key": keyPEM})
	if _, err := ExtractFromZIP(z); err == nil {
		t.Fatal("expected error for missing cert")
	}
}

func TestExtractFromZIPMissingKey(t *testing.T) {
	z := buildZIP(t, map[string]string{"a.crt": leafPEM})
	if _, err := ExtractFromZIP(z); err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestExtractFromZIPMultipleCertsPicksLongestChain(t *testing.T) {
	shortChain := "-----BEGIN CERTIFICATE-----\nshort\n-----END CERTIFICATE-----\n"
	longChain := "-----BEGIN CERTIFICATE-----\n" +
		strings.Repeat("longer-pem-content-", 50) +
		"\n-----END CERTIFICATE-----\n"
	z := buildZIPOrdered(t, []zipEntry{
		{Name: "leaf.crt", Body: leafPEM},
		{Name: "short.crt", Body: shortChain},
		{Name: "long.crt", Body: longChain},
		{Name: "priv.key", Body: keyPEM},
	})
	out, err := ExtractFromZIP(z)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !bytes.Contains(out.Chain, []byte("longer-pem-content-")) {
		t.Fatalf("expected longer chain selected, got %q", out.Chain)
	}
}
