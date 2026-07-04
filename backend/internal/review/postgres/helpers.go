package postgres

import (
	"time"
)

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func derefTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func stringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
