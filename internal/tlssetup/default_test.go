package tlssetup_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/tlssetup"
)

func TestDefaultCertBundleVerifies(t *testing.T) {
	bundle := tlssetup.DefaultCertBundle()

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(bundle.CACert) {
		t.Fatal("failed to parse CA cert PEM")
	}

	cert, err := tls.X509KeyPair(bundle.TLSCert, bundle.TLSKey)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}

	opts := x509.VerifyOptions{Roots: pool, DNSName: "kubesentry-gateway.kubesentry-system.svc"}
	if _, err := leaf.Verify(opts); err != nil {
		t.Errorf("cert verification failed: %v", err)
	}
}

func TestDefaultCertBundleValidFor90Years(t *testing.T) {
	bundle := tlssetup.DefaultCertBundle()

	block, _ := pem.Decode(bundle.TLSCert)
	if block == nil {
		t.Fatal("failed to decode server cert PEM")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}

	if time.Until(leaf.NotAfter) < 90*365*24*time.Hour {
		t.Errorf("expected a ~99-year cert, NotAfter is only %s away", time.Until(leaf.NotAfter))
	}
}
