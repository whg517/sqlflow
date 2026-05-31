package schema

import "time"

// timeNow returns the current time in UTC.
// Used as default value for time fields.
func timeNow() time.Time {
	return time.Now().UTC()
}
