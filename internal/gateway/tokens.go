// Package gateway implements the control-plane side of multi-cluster
// management: cluster registration, token auth, and heartbeat tracking.
package gateway

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Secret data keys shared by gateway and member sides.
const (
	KeyToken      = "token"
	KeyTokenHash  = "tokenHash"
	KeyClusterUID = "clusterUID"
)

// NewToken returns a 64-char hex token from crypto/rand (32 bytes entropy).
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// HashToken returns the SHA-256 hex of a token. The gateway persists only
// hashes; plaintext long-lived tokens live solely on the member cluster.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// RegTokenSecretName is the gateway-side Secret holding a cluster's one-time
// registration token (plaintext; short-lived, deleted on consumption).
func RegTokenSecretName(cluster string) string {
	return "kubesentry-reg-" + cluster
}

// AuthSecretName is the gateway-side Secret holding a cluster's long-lived
// token hash and bound clusterUID.
func AuthSecretName(cluster string) string {
	return "kubesentry-cluster-auth-" + cluster
}
