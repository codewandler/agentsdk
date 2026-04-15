package humanize_test

import (
	"testing"

	"github.com/codewandler/core/internal/humanize"
)

func TestSize(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  string
	}{
		{"zero", 0, "0B"},
		{"one byte", 1, "1B"},
		{"just under 1KB", 1023, "1023B"},
		{"exactly 1KB", 1024, "1.0KB"},
		{"1.5KB", 1536, "1.5KB"},
		{"just under 1MB", 1024*1024 - 1, "1024.0KB"},
		{"exactly 1MB", 1024 * 1024, "1.0MB"},
		{"2.5MB", int64(2.5 * 1024 * 1024), "2.5MB"},
		{"just under 1GB", 1024*1024*1024 - 1, "1024.0MB"},
		{"exactly 1GB", 1024 * 1024 * 1024, "1.0GB"},
		{"4GB", 4 * 1024 * 1024 * 1024, "4.0GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := humanize.Size(tt.input)
			if got != tt.want {
				t.Errorf("Size(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
