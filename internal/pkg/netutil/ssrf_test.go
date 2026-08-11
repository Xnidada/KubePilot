package netutil

import "testing"

func TestValidateOutboundURL(t *testing.T) {
	ok := []string{
		"https://hooks.slack.com/services/xxx",
		"http://example.com/hook",
	}
	for _, u := range ok {
		if err := ValidateOutboundURL(u); err != nil {
			t.Fatalf("%s: unexpected error: %v", u, err)
		}
	}

	bad := []string{
		"",
		"ftp://example.com",
		"http://127.0.0.1/x",
		"http://localhost/x",
		"http://10.0.0.1/x",
		"http://192.168.1.1/x",
		"http://169.254.169.254/latest",
		"http://[::1]/",
	}
	for _, u := range bad {
		if err := ValidateOutboundURL(u); err == nil {
			t.Fatalf("%s: expected error", u)
		}
	}
}
