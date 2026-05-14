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

func TestServerCertNotCA(t *testing.T) {
	bundle, err := tlssetup.GenerateCerts("webhook.example.com")
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

	// Server cert should NOT have IsCA flag
	if tlsCert.IsCA {
		t.Error("server cert should not have IsCA flag set")
	}
}

func TestMultipleDNSNames(t *testing.T) {
	dnsName := "my-webhook.default.svc.cluster.local"
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

	// Should have both the provided name and cluster.local variant
	if len(tlsCert.DNSNames) < 2 {
		t.Errorf("expected at least 2 DNS names, got %d", len(tlsCert.DNSNames))
	}

	// Check for both variants
	hasDNS := false
	hasClusterLocal := false
	for _, dn := range tlsCert.DNSNames {
		if dn == dnsName {
			hasDNS = true
		}
		if dn == dnsName+".cluster.local" {
			hasClusterLocal = true
		}
	}

	if !hasDNS {
		t.Errorf("missing DNS name %q", dnsName)
	}
	if !hasClusterLocal {
		t.Errorf("missing cluster.local DNS name")
	}
}

func TestCertKeyUsage(t *testing.T) {
	bundle, err := tlssetup.GenerateCerts("webhook.example.com")
	if err != nil {
		t.Fatalf("GenerateCerts: %v", err)
	}

	// Parse CA cert
	caBlock, _ := pem.Decode(bundle.CACert)
	caCert, _ := x509.ParseCertificate(caBlock.Bytes)

	// CA should have KeyUsageCertSign
	if caCert.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Error("CA cert should have KeyUsageCertSign")
	}
	if caCert.KeyUsage&x509.KeyUsageCRLSign == 0 {
		t.Error("CA cert should have KeyUsageCRLSign")
	}

	// Parse TLS cert
	tlsBlock, _ := pem.Decode(bundle.TLSCert)
	tlsCert, _ := x509.ParseCertificate(tlsBlock.Bytes)

	// Server should have KeyUsageDigitalSignature
	if tlsCert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Error("server cert should have KeyUsageDigitalSignature")
	}

	// Check ExtKeyUsage
	if len(tlsCert.ExtKeyUsage) == 0 {
		t.Error("server cert should have ExtKeyUsage")
	}
	found := false
	for _, usage := range tlsCert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageServerAuth {
			found = true
			break
		}
	}
	if !found {
		t.Error("server cert should have ExtKeyUsageServerAuth")
	}
}

func TestGenerateCertsConsistency(t *testing.T) {
	dnsName := "test.example.com"
	bundle1, err := tlssetup.GenerateCerts(dnsName)
	if err != nil {
		t.Fatalf("GenerateCerts: %v", err)
	}

	bundle2, err := tlssetup.GenerateCerts(dnsName)
	if err != nil {
		t.Fatalf("GenerateCerts: %v", err)
	}

	// Different invocations should produce different certs (due to randomness)
	if string(bundle1.CACert) == string(bundle2.CACert) {
		t.Error("two CA certs should be different due to randomness")
	}
	if string(bundle1.TLSCert) == string(bundle2.TLSCert) {
		t.Error("two TLS certs should be different due to randomness")
	}
}

func TestCASubjectCommonName(t *testing.T) {
	bundle, err := tlssetup.GenerateCerts("test.example.com")
	if err != nil {
		t.Fatalf("GenerateCerts: %v", err)
	}

	block, _ := pem.Decode(bundle.CACert)
	caCert, _ := x509.ParseCertificate(block.Bytes)

	if caCert.Subject.CommonName != "kubesentry-ca" {
		t.Errorf("CA CN = %q, want %q", caCert.Subject.CommonName, "kubesentry-ca")
	}
}

func TestServerSubjectCommonName(t *testing.T) {
	dnsName := "test.example.com"
	bundle, err := tlssetup.GenerateCerts(dnsName)
	if err != nil {
		t.Fatalf("GenerateCerts: %v", err)
	}

	block, _ := pem.Decode(bundle.TLSCert)
	tlsCert, _ := x509.ParseCertificate(block.Bytes)

	if tlsCert.Subject.CommonName != dnsName {
		t.Errorf("server CN = %q, want %q", tlsCert.Subject.CommonName, dnsName)
	}
}

func TestEmptyDNSName(t *testing.T) {
	bundle, err := tlssetup.GenerateCerts("")
	if err != nil {
		t.Fatalf("GenerateCerts with empty name should still work: %v", err)
	}

	// Even with empty DNS name, should get valid certificates
	if len(bundle.CACert) == 0 {
		t.Error("CACert should not be empty")
	}
	if len(bundle.TLSCert) == 0 {
		t.Error("TLSCert should not be empty")
	}
	if len(bundle.TLSKey) == 0 {
		t.Error("TLSKey should not be empty")
	}
}

func TestCertPEMFormat(t *testing.T) {
	bundle, err := tlssetup.GenerateCerts("test.example.com")
	if err != nil {
		t.Fatalf("GenerateCerts: %v", err)
	}

	// CACert should be valid PEM
	caBlock, caRest := pem.Decode(bundle.CACert)
	if caBlock == nil {
		t.Fatal("CACert PEM decode failed")
	}
	if len(caRest) > 0 {
		t.Error("CACert has extra data after certificate")
	}
	if caBlock.Type != "CERTIFICATE" {
		t.Errorf("CA cert PEM type = %q, want CERTIFICATE", caBlock.Type)
	}

	// TLSCert should be valid PEM
	tlsBlock, tlsRest := pem.Decode(bundle.TLSCert)
	if tlsBlock == nil {
		t.Fatal("TLSCert PEM decode failed")
	}
	if len(tlsRest) > 0 {
		t.Error("TLSCert has extra data after certificate")
	}
	if tlsBlock.Type != "CERTIFICATE" {
		t.Errorf("TLS cert PEM type = %q, want CERTIFICATE", tlsBlock.Type)
	}

	// TLSKey should be valid PEM
	keyBlock, keyRest := pem.Decode(bundle.TLSKey)
	if keyBlock == nil {
		t.Fatal("TLSKey PEM decode failed")
	}
	if len(keyRest) > 0 {
		t.Error("TLSKey has extra data after key")
	}
	if keyBlock.Type != "EC PRIVATE KEY" {
		t.Errorf("TLS key PEM type = %q, want EC PRIVATE KEY", keyBlock.Type)
	}
}

func TestSerialNumbers(t *testing.T) {
	bundle, err := tlssetup.GenerateCerts("test.example.com")
	if err != nil {
		t.Fatalf("GenerateCerts: %v", err)
	}

	// Parse CA cert
	caBlock, _ := pem.Decode(bundle.CACert)
	caCert, _ := x509.ParseCertificate(caBlock.Bytes)

	// Parse TLS cert
	tlsBlock, _ := pem.Decode(bundle.TLSCert)
	tlsCert, _ := x509.ParseCertificate(tlsBlock.Bytes)

	// Serial numbers should be different
	if caCert.SerialNumber.Cmp(tlsCert.SerialNumber) == 0 {
		t.Error("CA and TLS cert should have different serial numbers")
	}

	// Serial numbers should not be zero
	if caCert.SerialNumber.Sign() <= 0 {
		t.Error("CA serial number should be positive")
	}
	if tlsCert.SerialNumber.Sign() <= 0 {
		t.Error("TLS serial number should be positive")
	}
}
