package utilities

import (
	"testing"
)

func TestValidateAndNormalizeEthiopianPhone(t *testing.T) {
	tests := []struct {
		input       string
		expected    string
		shouldError bool
	}{
		{"0911223344", "+251911223344", false},
		{"0712345678", "+251712345678", false},
		{"+251911223344", "+251911223344", false},
		{"+251712345678", "+251712345678", false},
		{"251911223344", "+251911223344", false},
		{"251712345678", "+251712345678", false},
		{"911223344", "+251911223344", false},
		{"712345678", "+251712345678", false},
		{"09-11-22-33-44", "+251911223344", false},
		{"(0911) 223344", "+251911223344", false},
		{"+251 91 122 3344", "+251911223344", false},
		{"0811223344", "", true},
		{"0611223344", "", true},
		{"123456", "", true},
		{"091122334455", "", true},
		{"abcdefghij", "", true},
		{"", "", true},
	}

	for _, tc := range tests {
		got, err := ValidateAndNormalizeEthiopianPhone(tc.input)
		if tc.shouldError {
			if err == nil {
				t.Fatalf("expected error for input %q, got %q", tc.input, got)
			}
		} else {
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tc.input, err)
			}
			if got != tc.expected {
				t.Fatalf("for input %q: expected %q, got %q", tc.input, tc.expected, got)
			}
		}
	}
}
