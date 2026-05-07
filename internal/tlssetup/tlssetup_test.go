package tlssetup_test

import (
	"crypto/tls"
	"crypto/x509"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/tlssetup"
)

func TestGenerateCerts(t *testing.T) {
	bundle, err := tlssetup.GenerateCerts("kubesentry-webhook.kubesentry-system.svc")
	if err != nil {
		t.Fatalf("GenerateCerts: %v", err)
	}

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

	opts := x509.VerifyOptions{Roots: pool, DNSName: "kubesentry-webhook.kubesentry-system.svc"}
	if _, err := leaf.Verify(opts); err != nil {
		t.Errorf("cert verification failed: %v", err)
	}
}
