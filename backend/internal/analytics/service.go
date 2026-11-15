package analytics

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"site-assistant/internal/core"
	"strconv"
	"time"
)

type Filter struct {
	Start, End, Previous        time.Time
	Zone, Site, Assistant, Sort string
	Page                        int
}

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Dates are inclusive local calendar days; SQL receives a half-open UTC interval.
func Parse(q url.Values, now time.Time) (Filter, error) {
	f := Filter{Zone: q.Get("timezone"), Site: q.Get("site"), Assistant: q.Get("assistant"), Sort: q.Get("sort")}
	if f.Zone == "" {
		f.Zone = "UTC"
	}
	if f.Zone == "Local" || len(f.Zone) > 100 {
		return f, core.Fail("validation", 422)
	}
	loc, e := time.LoadLocation(f.Zone)
	if e != nil {
		return f, core.Fail("validation", 422)
	}
	start, end := q.Get("from"), q.Get("to")
	if start == "" && end == "" {
		end = now.In(loc).Format(time.DateOnly)
		start = now.In(loc).AddDate(0, 0, -6).Format(time.DateOnly)
	}
	a, e := time.ParseInLocation(time.DateOnly, start, loc)
	if e != nil {
		return f, core.Fail("validation", 422)
	}
	b, e := time.ParseInLocation(time.DateOnly, end, loc)
	if e != nil || b.Before(a) {
		return f, core.Fail("validation", 422)
	}
	days := 0
	for t := a; !t.After(b); t = t.AddDate(0, 0, 1) {
		days++
		if days > 93 {
			return f, core.Fail("validation", 422)
		}
	}
	f.Start = a.UTC()
	f.End = b.AddDate(0, 0, 1).UTC()
	f.Previous = a.AddDate(0, 0, -days).UTC()
	for _, id := range []string{f.Site, f.Assistant} {
		if id != "" && !uuid.MatchString(id) {
			return f, core.Fail("validation", 422)
		}
	}
	if f.Sort == "" {
		f.Sort = "dialogs"
	}
	if f.Sort != "dialogs" && f.Sort != "leads" && f.Sort != "handoffs" && f.Sort != "conversion" {
		return f, core.Fail("validation", 422)
	}
	if v := q.Get("page"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 0 || n > 1000 {
			return f, core.Fail("validation", 422)
		}
		f.Page = n
	}
	return f, nil
}

type Service struct {
	Auth  *core.Service
	Store *Store
}

func (s *Service) Report(ctx context.Context, user, org string, f Filter) (json.RawMessage, error) {
	if _, e := s.Auth.Access(ctx, user, org, true, false); e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return s.Store.Report(ctx, org, f)
}
