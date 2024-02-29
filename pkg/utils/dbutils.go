package utils

import (
	"database/sql"
	"time"
)

func NewSQLNullString(s string) sql.NullString {
	if len(s) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{
		String: s,
		Valid:  true,
	}
}

func GetTimeZone(t time.Time) int {
	_, offset := t.Zone()
	// Convert offset to hours and minutes
	hours := offset / 3600
	// minutes := (offset % 3600) / 60

	// // Format the timezone in a more readable form, e.g., "UTC+8"
	// eventTimezone := fmt.Sprintf("UTC%+d:%02d", hours, minutes)
	// return eventTimezone
	return hours
}
func ConvertTimeToLocal(t time.Time, offset time.Duration) time.Time {
	loca := time.FixedZone("UTC+8", int((offset * time.Hour).Seconds()))
	return t.In(loca)
}
