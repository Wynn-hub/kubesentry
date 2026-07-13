package hub

import "testing"

func TestNewTokenUniqueAndHex(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 64 || len(b) != 64 {
		t.Fatalf("token length = %d, %d; want 64", len(a), len(b))
	}
	if a == b {
		t.Fatal("two tokens are identical")
	}
	for _, r := range a {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Fatalf("non-hex rune %q in token", r)
		}
	}
}

func TestHashTokenDeterministic(t *testing.T) {
	h1 := HashToken("abc")
	h2 := HashToken("abc")
	if h1 != h2 {
		t.Fatal("hash not deterministic")
	}
	if h1 == HashToken("abd") {
		t.Fatal("different inputs share hash")
	}
	if len(h1) != 64 {
		t.Fatalf("hash length = %d, want 64 (sha256 hex)", len(h1))
	}
}

func TestSecretNames(t *testing.T) {
	if got := RegTokenSecretName("prod"); got != "kubesentry-reg-prod" {
		t.Fatalf("RegTokenSecretName = %q", got)
	}
	if got := AuthSecretName("prod"); got != "kubesentry-cluster-auth-prod" {
		t.Fatalf("AuthSecretName = %q", got)
	}
}
