package render

import (
	"fmt"
	"time"
)

// Age writes how long ago t was, as the queue and the review page show it:
// "now" under a minute, minutes under an hour, hours under a day, "Yesterday"
// under two days, days under a week, and the date after that.
//
// The queue tearout's note says a weekday after the days. A weekday for
// something more than a week old names the wrong week, so it is a date. A
// time after now, from a clock that moved back, is "now".
func Age(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	case d < 48*time.Hour:
		return "Yesterday"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
	t = t.In(now.Location())
	if t.Year() == now.Year() {
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2, 2006")
}
