package tlssetup

// DefaultCertBundle is a hardcoded 99-year self-signed CA + server cert for
// kubesentry-hub.kubesentry-system.svc, baked into this binary so a fresh
// hub install works with zero cert handling. Its private key ships in this
// repo — public to anyone with the source — so it is fine for local/dev use
// but must never be relied on for a real deployment. Configure the hub
// chart's tls.ca/cert/key values (charts/kubesentry-hub/values.yaml) to
// bring your own cert instead; that always takes precedence over this
// default, since it's only used when no cert has been persisted yet (see
// cmd/hub/main.go's ensureTLS).
func DefaultCertBundle() *CertBundle {
	return &CertBundle{
		CACert:  []byte(defaultCACertPEM),
		TLSCert: []byte(defaultTLSCertPEM),
		TLSKey:  []byte(defaultTLSKeyPEM),
	}
}

const defaultCACertPEM = `-----BEGIN CERTIFICATE-----
MIIBZDCCAQmgAwIBAgIBATAKBggqhkjOPQQDAjAYMRYwFAYDVQQDEw1rdWJlc2Vu
dHJ5LWNhMCAXDTI2MDcxNjAzMTE1NloYDzIxMjUwNjIyMDMxMjU2WjAYMRYwFAYD
VQQDEw1rdWJlc2VudHJ5LWNhMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEIvIZ
MDHH60/4SLTOnbStKs6o0EuEBJk5X6knHQ4RW7OnELkt2MBZktFQNbqTm11E3Rf2
TbYz4rrFvesy7U/lMaNCMEAwDgYDVR0PAQH/BAQDAgEGMA8GA1UdEwEB/wQFMAMB
Af8wHQYDVR0OBBYEFKZt957ElBDqorZytc6SgGqBe5eIMAoGCCqGSM49BAMCA0kA
MEYCIQDfCHcsxTBq0e0efbCZCPi1HHc1O7cx10ch8YSTrsM0iAIhAKuBD+6ev4CN
pXinc7YA55mCfRjLBKdPZNK64r2vlwES
-----END CERTIFICATE-----
`

const defaultTLSCertPEM = `-----BEGIN CERTIFICATE-----
MIIB5zCCAY2gAwIBAgIBAjAKBggqhkjOPQQDAjAYMRYwFAYDVQQDEw1rdWJlc2Vu
dHJ5LWNhMCAXDTI2MDcxNjAzMTE1NloYDzIxMjUwNjIyMDMxMjU2WjAvMS0wKwYD
VQQDEyRrdWJlc2VudHJ5LWh1Yi5rdWJlc2VudHJ5LXN5c3RlbS5zdmMwWTATBgcq
hkjOPQIBBggqhkjOPQMBBwNCAAR514EOxHnp8ZhWytBT7l8BgYOah8QNU921bAP4
QgG1mahPyb/5T9eJPtgPvlMGpil6rKaatM6D3mp7MRni3jaoo4GuMIGrMA4GA1Ud
DwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDATAfBgNVHSMEGDAWgBSmbfee
xJQQ6qK2crXOkoBqgXuXiDBjBgNVHREEXDBagiRrdWJlc2VudHJ5LWh1Yi5rdWJl
c2VudHJ5LXN5c3RlbS5zdmOCMmt1YmVzZW50cnktaHViLmt1YmVzZW50cnktc3lz
dGVtLnN2Yy5jbHVzdGVyLmxvY2FsMAoGCCqGSM49BAMCA0gAMEUCIG3hhQYFHFhD
V4sVCVM/XvmrqYE7FzwEmBy0vMXQuigKAiEArX2aLj0MiM5WngD6Wijfw0InIvpR
YHgJjB2/UM7NFuo=
-----END CERTIFICATE-----
`

const defaultTLSKeyPEM = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIBjy4FnAdYEwpbd7nvoxzzZEPFEtWs8drkWs4cts22ryoAoGCCqGSM49
AwEHoUQDQgAEedeBDsR56fGYVsrQU+5fAYGDmofEDVPdtWwD+EIBtZmoT8m/+U/X
iT7YD75TBqYpeqymmrTOg95qezEZ4t42qA==
-----END EC PRIVATE KEY-----
`
