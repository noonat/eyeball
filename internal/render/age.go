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
	return date(now, t)
}

// Ago writes the same spans as Age, worded to finish a sentence: "just now", "2
// hours ago", "yesterday", "on Sep 28". The tearouts use Age's form in a pill
// and this one in a notice.
func Ago(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d/time.Minute), "minute") + " ago"
	case d < 24*time.Hour:
		return plural(int(d/time.Hour), "hour") + " ago"
	case d < 48*time.Hour:
		return "yesterday"
	case d < 7*24*time.Hour:
		return plural(int(d/(24*time.Hour)), "day") + " ago"
	}
	return "on " + date(now, t)
}

// AgeAgo is Age's short form finishing a sentence, as the review tearout's
// verdict pill writes it: "just now", "2h ago", "yesterday", "on Sep 28".
func AgeAgo(now, t time.Time) string {
	switch a := Age(now, t); {
	case a == "now":
		return "just now"
	case a == "Yesterday":
		return "yesterday"
	case now.Sub(t) < 7*24*time.Hour:
		return a + " ago"
	default:
		return "on " + a
	}
}

// date is t's date as the clock showing now would show it, with the year when
// it is not now's.
func date(now, t time.Time) string {
	t = t.In(now.Location())
	if t.Year() == now.Year() {
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2, 2006")
}

// plural writes n with its noun.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
