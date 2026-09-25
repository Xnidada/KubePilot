package model

import "testing"

func TestValidateSeedPassword(t *testing.T) {
	for _, password := range []string{"", "admin123", "short-pass"} {
		if err := validateSeedPassword(password); err == nil {
			t.Errorf("accepted weak bootstrap password %q", password)
		}
	}
	if err := validateSeedPassword("correct-horse-2026!"); err != nil {
		t.Fatalf("rejected strong password: %v", err)
	}
}
