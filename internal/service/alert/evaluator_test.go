package alert

import "testing"

func TestParseDuration(t *testing.T) {
	cases := []struct {
		in      string
		wantSec int64
		wantErr bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"5m", 300, false},
		{"1h", 3600, false},
		{"30s", 30, false},
		{"bad", 0, true},
	}
	for _, tc := range cases {
		d, err := parseDuration(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%q: expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", tc.in, err)
		}
		if int64(d.Seconds()) != tc.wantSec {
			t.Fatalf("%q: got %v want %ds", tc.in, d, tc.wantSec)
		}
	}
}
