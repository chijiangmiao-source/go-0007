package domain

import (
	"strings"
	"time"
)

var ccsdsLayouts = []string{
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05.000Z",
	"2006-01-02T15:04:05.000000Z",
	"2006-01-02T15:04:05.000000000Z",
	time.RFC3339Nano,
}

func ParseCCSDSTime(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, NewError(CodeValidation, "ccsds time is required")
	}
	if !(strings.HasSuffix(raw, "Z") || strings.HasSuffix(raw, "+00:00")) {
		return time.Time{}, Errorf(CodeValidation, "ccsds time %q must include explicit UTC marker", raw)
	}
	for _, layout := range ccsdsLayouts {
		t, err := time.Parse(layout, raw)
		if err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, Errorf(CodeValidation, "ccsds time %q is not parseable", raw)
}

func NowUTC() time.Time {
	return time.Now().UTC().Truncate(time.Nanosecond)
}
