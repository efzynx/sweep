package core

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

const (
	Byte int64 = 1
	KB   int64 = 1024 * Byte
	MB   int64 = 1024 * KB
	GB   int64 = 1024 * MB
	TB   int64 = 1024 * GB
	PB   int64 = 1024 * TB
	EB   int64 = 1024 * PB
)

var unitScale = map[string]int64{
	"B":   Byte,
	"BYTES": Byte,
	"BYTE":  Byte,
	"K":   KB,
	"KB":  KB,
	"KIB": KB,
	"M":   MB,
	"MB":  MB,
	"MIB": MB,
	"G":   GB,
	"GB":  GB,
	"GIB": GB,
	"T":   TB,
	"TB":  TB,
	"TIB": TB,
	"P":   PB,
	"PB":  PB,
	"PIB": PB,
	"E":   EB,
	"EB":  EB,
	"EIB": EB,
}

// FormatBytes formats a byte count into a human-readable string (B, KB, MB, GB, TB, PB, EB).
func FormatBytes(bytes int64) string {
	if bytes < 0 {
		return "-" + FormatBytes(-bytes)
	}

	switch {
	case bytes < KB:
		return fmt.Sprintf("%d B", bytes)
	case bytes < MB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	case bytes < GB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes < TB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes < PB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/float64(TB))
	default:
		return fmt.Sprintf("%.2f PB", float64(bytes)/float64(PB))
	}
}

// ParseBytes parses a human-readable byte string (e.g., "500 B", "1.5 MB", "10GB", "2.5GiB")
// into an int64 byte count.
func ParseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty byte string")
	}

	isNegative := false
	if strings.HasPrefix(s, "-") {
		isNegative = true
		s = strings.TrimSpace(s[1:])
	} else if strings.HasPrefix(s, "+") {
		s = strings.TrimSpace(s[1:])
	}

	if s == "" {
		return 0, errors.New("invalid byte string")
	}

	// Split numeric part and unit part
	var numEnd int
	for numEnd < len(s) {
		r := rune(s[numEnd])
		if !unicode.IsDigit(r) && r != '.' && r != 'e' && r != 'E' {
			break
		}
		numEnd++
	}

	numStr := strings.TrimSpace(s[:numEnd])
	unitStr := strings.ToUpper(strings.TrimSpace(s[numEnd:]))

	if numStr == "" {
		return 0, fmt.Errorf("no numeric value in %q", s)
	}

	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid byte number %q: %w", numStr, err)
	}

	var mult int64 = 1
	if unitStr != "" {
		scale, ok := unitScale[unitStr]
		if !ok {
			return 0, fmt.Errorf("unknown byte unit %q", unitStr)
		}
		mult = scale
	}

	total := int64(val * float64(mult))
	if isNegative {
		total = -total
	}

	return total, nil
}
