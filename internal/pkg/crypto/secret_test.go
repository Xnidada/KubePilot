package crypto

import "testing"

func TestSecretSealOpenAndLegacy(t *testing.T) {
	const key = "long-test-encryption-key"
	sealed, err := SealSecret("token-value", key)
	if err != nil || sealed == "token-value" {
		t.Fatalf("seal: %q %v", sealed, err)
	}
	if got, err := OpenSecret(sealed, key); err != nil || got != "token-value" {
		t.Fatalf("open: %q %v", got, err)
	}
	if got, err := SealSecret(sealed, key); err != nil || got != sealed {
		t.Fatalf("reseal: %q %v", got, err)
	}
	if got, err := OpenSecret("legacy", key); err != nil || got != "legacy" {
		t.Fatalf("legacy: %q %v", got, err)
	}
	if _, err := OpenSecret(sealed, "wrong-key"); err == nil {
		t.Fatal("wrong key accepted")
	}
}
