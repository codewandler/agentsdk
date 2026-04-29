// Package toolmw provides concrete middleware implementations for the
// agentsdk tool system: timeout, risk gating, secret protection, etc.
package toolmw

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// parseDuration parses a human-friendly duration string.
// It extends time.ParseDuration with support for:
//   - "m" as minutes (time.ParseDuration treats "m" as minutes already)
//   - "min" / "mins" as minutes
//   - "s" / "sec" / "secs" as seconds
//   - "h" / "hr" / "hrs" as hours
//   - bare integers treated as seconds (e.g. "30" → 30s)
//   - compound forms like "2m30s", "1h30m"
//
// Examples: "30s", "2m", "5m", "1.5h", "90", "2min", "1h30m", "30sec"
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration string")
	}

	// Try standard library first — handles "30s", "2m", "1h30m", "1.5h", etc.
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Bare number → seconds. Reject negative values.
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		if n < 0 {
			return 0, fmt.Errorf("negative duration %q", s)
		}
		return time.Duration(n * float64(time.Second)), nil
	}

	// Try human-friendly suffixes.
	lower := strings.ToLower(s)
	if d, ok := parseHumanSuffix(lower); ok {
		return d, nil
	}

	return 0, fmt.Errorf("invalid duration %q", s)
}

var humanDurationPattern = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(sec|secs|second|seconds|min|mins|minute|minutes|hr|hrs|hour|hours)$`)

func parseHumanSuffix(s string) (time.Duration, bool) {
	m := humanDurationPattern.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}

	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}

	var unit time.Duration
	switch m[2] {
	case "sec", "secs", "second", "seconds":
		unit = time.Second
	case "min", "mins", "minute", "minutes":
		unit = time.Minute
	case "hr", "hrs", "hour", "hours":
		unit = time.Hour
	default:
		return 0, false
	}

	return time.Duration(n * float64(unit)), true
}
