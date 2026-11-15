package analytics

import (
	"net/url"
	"testing"
	"time"
)

func TestCalendarRanges(t *testing.T) {
	for _, zone := range []string{"UTC", "Europe/Prague", "Europe/Berlin", "Europe/Moscow"} {
		f, e := Parse(url.Values{"from": {"2026-03-29"}, "to": {"2026-03-29"}, "timezone": {zone}}, time.Now())
		if e != nil {
			t.Fatal(e)
		}
		hours := f.End.Sub(f.Start).Hours()
		if zone == "Europe/Prague" && hours != 23 {
			t.Fatal(hours)
		}
	}
	for _, q := range []url.Values{{"from": {"2026-02-30"}}, {"from": {"2026-01-01"}, "to": {"2026-12-31"}}, {"timezone": {"bad"}}, {"site": {"bad"}}, {"page": {"-1"}}} {
		if _, e := Parse(q, time.Now()); e == nil {
			t.Fatal(q)
		}
	}
}
