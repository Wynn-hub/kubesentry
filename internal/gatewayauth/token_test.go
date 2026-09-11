package gatewayauth

import "testing"

func TestTokenPrimitives(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 64 || len(b) != 64 || a == b {
		t.Fatalf("invalid tokens: lengths %d/%d, equal=%t", len(a), len(b), a == b)
	}
	for _, r := range a {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Fatalf("non-hex rune %q in token", r)
		}
	}
	if HashToken("abc") != HashToken("abc") || HashToken("abc") == HashToken("abd") || len(HashToken("abc")) != 64 {
		t.Fatal("SHA-256 token hash contract changed")
	}
}
