package tlssetup_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

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

func TestCertBundleFieldsNonEmpty(t *testing.T) {
	bundle, err := tlssetup.GenerateCerts("test.example.com")
	if err != nil {
		t.Fatalf("GenerateCerts: %v", err)
	}

	if len(bundle.CACert) == 0 {
		t.Error("CACert is empty")
	}
	if len(bundle.TLSCert) == 0 {
		t.Error("TLSCert is empty")
	}
	if len(bundle.TLSKey) == 0 {
		t.Error("TLSKey is empty")
	}
}

func TestCACertIsSelfSigned(t *testing.T) {
	bundle, err := tlssetup.GenerateCerts("test.example.com")
	if err != nil {
		t.Fatalf("GenerateCerts: %v", err)
	}

	block, _ := pem.Decode(bundle.CACert)
	if block == nil {
		t.Fatal("failed to decode CA cert PEM")
	}

	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}

	// Self-signed cert has Issuer == Subject
	if caCert.Issuer.String() != caCert.Subject.String() {
		t.Error("CA cert is not self-signed (Issuer != Subject)")
	}

	// CA cert should have IsCA flag
	if !caCert.IsCA {
		t.Error("CA cert should have IsCA flag set")
	}
}

func TestServerCertDNSNames(t *testing.T) {
	dnsName := "webhook.example.svc.cluster.local"
	bundle, err := tlssetup.GenerateCerts(dnsName)
	if err != nil {
		t.Fatalf("GenerateCerts: %v", err)
	}

	block, _ := pem.Decode(bundle.TLSCert)
	if block == nil {
		t.Fatal("failed to decode TLS cert PEM")
	}

	tlsCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}

	// Server cert should have the provided DNS name in DNSNames
	found := false
	for _, dn := range tlsCert.DNSNames {
		if dn == dnsName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DNS name %q not found in cert DNSNames: %v", dnsName, tlsCert.DNSNames)
	}
}

func TestCertExpiry(t *testing.T) {
	bundle, err := tlssetup.GenerateCerts("webhook.example.com")
	if err != nil {
		t.Fatalf("GenerateCerts: %v", err)
	}

	block, _ := pem.Decode(bundle.TLSCert)
	if block == nil {
		t.Fatal("failed to decode TLS cert PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}

	// Check that cert is valid for approximately 365 days (allow 370 days max due to clock skew)
	now := time.Now()
	daysValid := cert.NotAfter.Sub(cert.NotBefore).Hours() / 24
	if daysValid < 360 || daysValid > 370 {
		t.Errorf("cert validity = %.0f days, want ~365 days", daysValid)
	}

	// Cert should not be expired now
	if now.After(cert.NotAfter) {
		t.Error("cert is already expired")
	}

	// Cert should not be valid before NotBefore
	if now.Before(cert.NotBefore) {
		t.Error("cert is not yet valid")
	}
}

func TestServerCertSignedByCA(t *testing.T) {
	bundle, err := tlssetup.GenerateCerts("webhook.example.com")
	if err != nil {
		t.Fatalf("GenerateCerts: %v", err)
	}

	// Parse CA cert
	caBlock, _ := pem.Decode(bundle.CACert)
	if caBlock == nil {
		t.Fatal("failed to decode CA cert PEM")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate CA: %v", err)
	}

	// Parse TLS cert
	tlsBlock, _ := pem.Decode(bundle.TLSCert)
	if tlsBlock == nil {
		t.Fatal("failed to decode TLS cert PEM")
	}
	tlsCert, err := x509.ParseCertificate(tlsBlock.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate TLS: %v", err)
	}

	// Verify TLS cert is signed by CA
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	opts := x509.VerifyOptions{Roots: pool}
	if _, err := tlsCert.Verify(opts); err != nil {
		t.Errorf("TLS cert not signed by CA: %v", err)
	}
}
