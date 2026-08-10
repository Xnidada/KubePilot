package config

import "testing"

func TestEncryptKeyPrefersSecurityKey(t *testing.T) {
	cfg := &Config{
		JWT:      JWTConfig{Secret: "jwt-secret-at-least-16"},
		Security: SecurityConfig{EncryptKey: "encrypt-key-at-least-16"},
	}
	if got := cfg.EncryptKey(); got != "encrypt-key-at-least-16" {
		t.Fatalf("EncryptKey() = %q, want security.encrypt_key", got)
	}
}

func TestEncryptKeyFallsBackToJWT(t *testing.T) {
	cfg := &Config{
		JWT: JWTConfig{Secret: "jwt-secret-at-least-16"},
	}
	if got := cfg.EncryptKey(); got != "jwt-secret-at-least-16" {
		t.Fatalf("EncryptKey() = %q, want jwt.secret fallback", got)
	}
}

func TestValidateRejectsShortEncryptKey(t *testing.T) {
	cfg := &Config{
		JWT:      JWTConfig{Secret: "jwt-secret-at-least-16"},
		Security: SecurityConfig{EncryptKey: "short"},
		Database: DatabaseConfig{Host: "localhost", Driver: "postgres"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for short encrypt_key")
	}
}
