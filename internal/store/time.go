package store

import (
	"fmt"
	"strings"
	"time"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func parseTimeValue(value any) (time.Time, error) {
	switch v := value.(type) {
	case nil:
		return time.Time{}, nil
	case time.Time:
		return v, nil
	case string:
		return parseTimeString(v)
	case []byte:
		return parseTimeString(string(v))
	default:
		return time.Time{}, fmt.Errorf("unsupported time value type %T", value)
	}
}

func parseTimeString(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}

	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts, nil
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts, nil
	}
	if ts, err := time.Parse(time.RubyDate, value); err == nil {
		return ts, nil
	}
	if ts, err := time.Parse(time.ANSIC, value); err == nil {
		return ts, nil
	}
	if ts, err := time.Parse(time.UnixDate, value); err == nil {
		return ts, nil
	}
	if ts, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", value); err == nil {
		return ts, nil
	}

	return time.Time{}, fmt.Errorf("invalid time format: %q", value)
}
