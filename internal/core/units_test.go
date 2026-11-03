package core

import (
	"testing"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{name: "zero", input: 0, expected: "0 B"},
		{name: "small bytes", input: 512, expected: "512 B"},
		{name: "exact 1 KB", input: 1024, expected: "1.00 KB"},
		{name: "fractional KB", input: 1536, expected: "1.50 KB"},
		{name: "exact 1 MB", input: 1024 * 1024, expected: "1.00 MB"},
		{name: "fractional MB", input: 2621440, expected: "2.50 MB"}, // 2.5 * 1024 * 1024
		{name: "exact 1 GB", input: 1024 * 1024 * 1024, expected: "1.00 GB"},
		{name: "fractional GB", input: int64(1.5 * float64(GB)), expected: "1.50 GB"},
		{name: "exact 1 TB", input: TB, expected: "1.00 TB"},
		{name: "exact 1 PB", input: PB, expected: "1.00 PB"},
		{name: "negative value", input: -1536, expected: "-1.50 KB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatBytes(tt.input)
			if got != tt.expected {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseBytes(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    int64
		expectError bool
	}{
		{name: "zero", input: "0 B", expected: 0},
		{name: "zero plain", input: "0", expected: 0},
		{name: "bytes plain", input: "500", expected: 500},
		{name: "bytes with unit", input: "500 B", expected: 500},
		{name: "bytes no space", input: "500B", expected: 500},
		{name: "lowercase b", input: "500b", expected: 500},
		{name: "1 KB", input: "1KB", expected: 1024},
		{name: "1.5 KB", input: "1.5 KB", expected: 1536},
		{name: "1.5 KiB", input: "1.5 KiB", expected: 1536},
		{name: "10 MB", input: "10 MB", expected: 10 * MB},
		{name: "2.5 GB", input: "2.5 GB", expected: int64(2.5 * float64(GB))},
		{name: "1 TB", input: "1 TB", expected: TB},
		{name: "negative MB", input: "-2 MB", expected: -2 * MB},
		{name: "empty string", input: "", expectError: true},
		{name: "invalid text", input: "invalid", expectError: true},
		{name: "unknown unit", input: "100 XYZ", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBytes(tt.input)
			if tt.expectError {
				if err == nil {
					t.Fatalf("ParseBytes(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseBytes(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.expected {
				t.Errorf("ParseBytes(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFormatAndParseRoundtrip(t *testing.T) {
	values := []int64{0, 1024, 2048, 10 * MB, 5 * GB, 2 * TB}
	for _, v := range values {
		formatted := FormatBytes(v)
		parsed, err := ParseBytes(formatted)
		if err != nil {
			t.Fatalf("ParseBytes(%q) failed: %v", formatted, err)
		}
		if parsed != v {
			t.Errorf("Roundtrip mismatch: original %d -> formatted %q -> parsed %d", v, formatted, parsed)
		}
	}
}
