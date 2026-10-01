package utilities

import "testing"

func TestNormalizeCBEAccount(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"1000123456789", "1000123456789", false},
		{" 1000 1234 56789 ", "1000123456789", false},
		{"1000-1234-56789", "1000123456789", false},
		{"100012345678", "", true},
		{"10001234567890", "", true},
		{"10001234567a9", "", true},
		{"", "", true},
	}
	for _, tc := range tests {
		got, err := NormalizeCBEAccount(tc.input)
		if tc.wantErr != (err != nil) || got != tc.want {
			t.Errorf("NormalizeCBEAccount(%q) = %q, %v; want %q, error %v", tc.input, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestNormalizeTelebirrAccount(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"0911223344", "0911223344", false},
		{"+251911223344", "0911223344", false},
		{"251911223344", "0911223344", false},
		{"911223344", "0911223344", false},
		{"091 122 3344", "0911223344", false},
		{"0711223344", "", true},
		{"091122334", "", true},
		{"09112233445", "", true},
		{"", "", true},
	}
	for _, tc := range tests {
		got, err := NormalizeTelebirrAccount(tc.input)
		if tc.wantErr != (err != nil) || got != tc.want {
			t.Errorf("NormalizeTelebirrAccount(%q) = %q, %v; want %q, error %v", tc.input, got, err, tc.want, tc.wantErr)
		}
	}
}
