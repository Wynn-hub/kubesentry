package tlssetup

// DefaultServerName is the DNS SAN baked into DefaultCertBundle's server
// cert. Clients falling back to the default CA (no custom
// gateway.caBundle/tls.ca configured) must use this as the TLS server name
// unless the gateway runs in a non-default namespace.
const DefaultServerName = "kubesentry-gateway.kubesentry-system.svc"

// DefaultCertBundle is a hardcoded 99-year self-signed CA + server cert for
// kubesentry-gateway.kubesentry-system.svc, baked into this binary so a
// fresh gateway install works with zero cert handling. Its private key ships
// in this repo — public to anyone with the source — so it is fine for
// local/dev use but must never be relied on for a real deployment. Configure
// the gateway chart's tls.ca/cert/key values
// (charts/kubesentry-gateway/values.yaml) to bring your own cert instead;
// that always takes precedence over this default, since it's only used when
// no cert has been persisted yet (see cmd/gateway/main.go's ensureTLS).
func DefaultCertBundle() *CertBundle {
	return &CertBundle{
		CACert:  []byte(defaultCACertPEM),
		TLSCert: []byte(defaultTLSCertPEM),
		TLSKey:  []byte(defaultTLSKeyPEM),
	}
}

const defaultCACertPEM = `-----BEGIN CERTIFICATE-----
MIIBYjCCAQmgAwIBAgIBATAKBggqhkjOPQQDAjAYMRYwFAYDVQQDEw1rdWJlc2Vu
dHJ5LWNhMCAXDTI2MDcxNzAxNDE0NVoYDzIxMjUwNjIzMDE0MjQ1WjAYMRYwFAYD
VQQDEw1rdWJlc2VudHJ5LWNhMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE4Dbj
txFNfKx8wRnWWPMCoxfLXuF4cUTVkpHUsFd6zROyTWRooYneUX0wc8UXGoFBZpmD
4ApFoSNCaHLpZLn/JqNCMEAwDgYDVR0PAQH/BAQDAgEGMA8GA1UdEwEB/wQFMAMB
Af8wHQYDVR0OBBYEFPTVjs6vCY3t78l2eaHFsVXzBaj9MAoGCCqGSM49BAMCA0cA
MEQCIFqlZIv1Sh+F+dUYYaSFEHS6+uGYrzgsKrD+/CQlDdHOAiBOemnrG0PGbB6m
0rCgy5fkrMMZ4gUJTaO//lSot/FeDg==
-----END CERTIFICATE-----
`

const defaultTLSCertPEM = `-----BEGIN CERTIFICATE-----
MIIB9DCCAZmgAwIBAgIBAjAKBggqhkjOPQQDAjAYMRYwFAYDVQQDEw1rdWJlc2Vu
dHJ5LWNhMCAXDTI2MDcxNzAxNDE0NVoYDzIxMjUwNjIzMDE0MjQ1WjAzMTEwLwYD
VQQDEyhrdWJlc2VudHJ5LWdhdGV3YXkua3ViZXNlbnRyeS1zeXN0ZW0uc3ZjMFkw
EwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEyGsOXm4jJJQy/BaNWpDLI6Z53KJrG0bR
IrjuNyby8qp1xVRoV6pd4D8ji6Q5a+Xj119GTFJgQlRKOFf+Db8ff6OBtjCBszAO
BgNVHQ8BAf8EBAMCB4AwEwYDVR0lBAwwCgYIKwYBBQUHAwEwHwYDVR0jBBgwFoAU
9NWOzq8Jje3vyXZ5ocWxVfMFqP0wawYDVR0RBGQwYoIoa3ViZXNlbnRyeS1nYXRl
d2F5Lmt1YmVzZW50cnktc3lzdGVtLnN2Y4I2a3ViZXNlbnRyeS1nYXRld2F5Lmt1
YmVzZW50cnktc3lzdGVtLnN2Yy5jbHVzdGVyLmxvY2FsMAoGCCqGSM49BAMCA0kA
MEYCIQDBSYeUNnRH1aEIDxHrn4da5w3mssibsLVNzIt1gdwjawIhAOphLZQPA10U
XW8YXb3uJfEIMYVfPHqjSUO5611FKvHv
-----END CERTIFICATE-----
`

const defaultTLSKeyPEM = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIDgTaof9Og8Yb5kqWYGQiaZjW1f2lD3sbSeBNul4zLZgoAoGCCqGSM49
AwEHoUQDQgAEyGsOXm4jJJQy/BaNWpDLI6Z53KJrG0bRIrjuNyby8qp1xVRoV6pd
4D8ji6Q5a+Xj119GTFJgQlRKOFf+Db8ffw==
-----END EC PRIVATE KEY-----
`
