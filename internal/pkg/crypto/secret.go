package crypto

import "strings"

const secretPrefix = "enc:v1:"

// SealSecret marks encrypted values so pre-existing plaintext rows can be migrated.
func SealSecret(value, key string) (string, error) {
	if value == "" || strings.HasPrefix(value, secretPrefix) {
		return value, nil
	}
	sealed, err := Encrypt(value, key)
	if err != nil {
		return "", err
	}
	return secretPrefix + sealed, nil
}

func OpenSecret(value, key string) (string, error) {
	if !strings.HasPrefix(value, secretPrefix) {
		return value, nil // legacy plaintext, migrated at startup
	}
	return Decrypt(strings.TrimPrefix(value, secretPrefix), key)
}
