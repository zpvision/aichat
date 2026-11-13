package leads

import (
	"bytes"
	"encoding/json"
	"math"
	"net/mail"
	"regexp"
	"site-assistant/internal/core"
	"sort"
	"strings"
	"unicode/utf8"
)

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

func validText(v string, n int) bool {
	return utf8.ValidString(v) && utf8.RuneCountInString(v) <= n && !strings.ContainsRune(v, 0)
}
func locale(v string) bool { return v == "ru" || v == "en" || v == "cs" || v == "de" }
func ValidateSettings(s *Settings) error {
	if !validText(s.Title, 120) || !validText(s.Goal, 1500) || len(s.Fields) > 12 || s.Enabled && len(s.Fields) == 0 {
		return core.Fail("validation", 422)
	}
	for l, v := range s.Success {
		if !locale(l) || !validText(v, 400) {
			return core.Fail("validation", 422)
		}
	}
	seen := map[string]bool{}
	contact, active := false, 0
	for i := range s.Fields {
		f := &s.Fields[i]
		if !keyPattern.MatchString(f.Key) || seen[f.Key] || !validText(f.Label, 100) || (labels[f.Key] == nil && strings.TrimSpace(f.Label) == "") || !validText(f.Description, 400) || f.Position < 0 || f.Position > 1000 {
			return core.Fail("validation", 422)
		}
		seen[f.Key] = true
		for l, v := range f.Labels {
			if !locale(l) || !validText(v, 100) {
				return core.Fail("validation", 422)
			}
		}
		switch f.Type {
		case "text":
			if f.MaxLength == 0 {
				f.MaxLength = 500
			}
			if f.MinLength < 0 || f.MaxLength < 1 || f.MaxLength > 2000 || f.MinLength > f.MaxLength {
				return core.Fail("validation", 422)
			}
		case "email", "phone":
			if f.Active {
				contact = true
			}
			if s.RequireContact && f.Required {
				return core.Fail("lead_contact_rule", 422)
			}
		case "inn_ru":
		case "number":
			for _, p := range []*float64{f.Min, f.Max} {
				if p != nil && (math.IsNaN(*p) || math.IsInf(*p, 0) || *p < -1e9 || *p > 1e9) {
					return core.Fail("validation", 422)
				}
			}
			if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
				return core.Fail("validation", 422)
			}
		case "select":
			if len(f.Options) < 1 || len(f.Options) > 30 {
				return core.Fail("validation", 422)
			}
			options := map[string]bool{}
			for _, v := range f.Options {
				if !validText(v, 100) || strings.TrimSpace(v) == "" || options[v] {
					return core.Fail("validation", 422)
				}
				options[v] = true
			}
		default:
			return core.Fail("validation", 422)
		}
		if f.Active {
			active++
		}
	}
	if s.Enabled && (active == 0 || s.RequireContact && !contact) {
		return core.Fail("lead_contact_rule", 422)
	}
	sort.SliceStable(s.Fields, func(i, j int) bool { return s.Fields[i].Position < s.Fields[j].Position })
	return nil
}
func ValidateValue(f Field, raw json.RawMessage) (json.RawMessage, string) {
	raw = bytes.TrimSpace(raw)
	if len(raw) > 10000 || !json.Valid(raw) {
		return json.RawMessage(`null`), "lead_value_type"
	}
	if f.Type == "number" {
		var n float64
		if json.Unmarshal(raw, &n) != nil || bytes.Equal(raw, []byte("null")) || math.IsNaN(n) || math.IsInf(n, 0) {
			return raw, "lead_value_type"
		}
		if n < -1e9 || n > 1e9 || f.Min != nil && n < *f.Min || f.Max != nil && n > *f.Max || f.Integer && n != math.Trunc(n) {
			return raw, "lead_value_range"
		}
		b, _ := json.Marshal(n)
		return b, ""
	}
	var v string
	if json.Unmarshal(raw, &v) != nil || bytes.Equal(raw, []byte("null")) {
		return raw, "lead_value_type"
	}
	v = strings.TrimSpace(v)
	if !validText(v, 2000) {
		return json.RawMessage(`""`), "lead_value_length"
	}
	code := ""
	switch f.Type {
	case "text":
		if utf8.RuneCountInString(v) < f.MinLength || utf8.RuneCountInString(v) > f.MaxLength || v == "" {
			code = "lead_value_length"
		}
	case "email":
		v = strings.ToLower(v)
		a, e := mail.ParseAddress(v)
		if e != nil || a.Address != v || len(v) > 254 || !strings.Contains(strings.Split(v, "@")[len(strings.Split(v, "@"))-1], ".") {
			code = "lead_value_email"
		}
	case "phone":
		v = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", ".", "").Replace(v)
		if len(v) < 9 || len(v) > 16 || v[0] != '+' || v[1] == '0' || !digits(v[1:]) {
			code = "lead_value_phone"
		}
	case "inn_ru":
		if !ValidINN(v) {
			code = "lead_value_inn"
		}
	case "select":
		found := false
		for _, o := range f.Options {
			if v == o {
				found = true
			}
		}
		if !found {
			code = "lead_value_select"
		}
	default:
		code = "lead_value_type"
	}
	b, _ := json.Marshal(v)
	return b, code
}
func digits(v string) bool {
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return v != ""
}
func ValidINN(s string) bool {
	if !digits(s) || strings.Trim(s, "0") == "" {
		return false
	}
	check := func(weights []int, n int) bool {
		sum := 0
		for i, w := range weights {
			sum += int(s[i]-'0') * w
		}
		return sum%11%10 == int(s[n]-'0')
	}
	if len(s) == 10 {
		return check([]int{2, 4, 10, 3, 5, 9, 4, 6, 8}, 9)
	}
	return len(s) == 12 && check([]int{7, 2, 4, 10, 3, 5, 9, 4, 6, 8}, 10) && check([]int{3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8}, 11)
}
func Missing(s Settings, values map[string]Value) []string {
	out := []string{}
	contact := false
	for _, f := range s.Fields {
		if !f.Active {
			continue
		}
		v, ok := values[f.Key]
		if f.Required && (!ok || !v.Valid) || ok && !v.Valid {
			out = append(out, f.Key)
		}
		if (f.Type == "email" || f.Type == "phone") && ok && v.Valid {
			contact = true
		}
	}
	if s.RequireContact && !contact {
		out = append(out, "$contact")
	}
	if len(values) == 0 && len(out) == 0 {
		for _, f := range s.Fields {
			if f.Active {
				out = append(out, f.Key)
				break
			}
		}
	}
	return out
}
