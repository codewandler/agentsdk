package toolmw

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		// Standard Go durations
		{"30s", 30 * time.Second},
		{"2m", 2 * time.Minute},
		{"1h", time.Hour},
		{"1h30m", 90 * time.Minute},
		{"1.5h", 90 * time.Minute},
		{"500ms", 500 * time.Millisecond},
		{"2m30s", 150 * time.Second},

		// Bare integers → seconds
		{"30", 30 * time.Second},
		{"90", 90 * time.Second},
		{"0", 0},

		// Bare floats → seconds
		{"1.5", 1500 * time.Millisecond},

		// Human-friendly suffixes
		{"30sec", 30 * time.Second},
		{"30secs", 30 * time.Second},
		{"30second", 30 * time.Second},
		{"30seconds", 30 * time.Second},
		{"2min", 2 * time.Minute},
		{"2mins", 2 * time.Minute},
		{"2minute", 2 * time.Minute},
		{"2minutes", 2 * time.Minute},
		{"1hr", time.Hour},
		{"1hrs", time.Hour},
		{"1hour", time.Hour},
		{"2hours", 2 * time.Hour},

		// Whitespace tolerance
		{"  30s  ", 30 * time.Second},
		{" 2min ", 2 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseDuration_Errors(t *testing.T) {
	tests := []string{
		"",
		"abc",
		"forever",
		"--5s",
		"2x",
		"-5",
		"-1.5",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := parseDuration(input)
			require.Error(t, err)
		})
	}
}
