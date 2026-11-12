package core

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/net/idna"
	"regexp"
	"site-assistant/internal/config"
	"strings"
	"time"
)

type Service struct {
	Store     *Store
	dummyHash string
}

func NewService(s *Store) *Service {
	return &Service{Store: s, dummyHash: HashPassword("timing-only-unusable-password")}
}
func validLocale(v string) bool { return config.Locale(v) }
func isUnique(e error) bool     { var p *pgconn.PgError; return errors.As(e, &p) && p.Code == "23505" }
func TranslateError(e error) error {
	if e == nil {
		return nil
	}
	var f *Fault
	if errors.As(e, &f) {
		return e
	}
	if errors.Is(e, pgx.ErrNoRows) {
		return Fail("not_found", 404)
	}
	var p *pgconn.PgError
	if errors.As(e, &p) {
		if p.Code == "23505" {
			return Fail("conflict", 409)
		}
		if p.Code == "22P02" || p.Code == "23503" || p.Code == "23514" {
			return Fail("validation", 422)
		}
	}
	return Fail("internal", 500)
}
func (s *Service) Access(ctx context.Context, user, org string, write, owner bool) (string, error) {
	if !uuid.MatchString(org) {
		return "", Fail("not_found", 404)
	}
	role, e := s.Store.Role(ctx, org, user)
	if e != nil {
		return "", TranslateError(e)
	}
	if (owner && role != "owner") || (write && role == "operator") {
		return "", Fail("forbidden", 403)
	}
	return role, nil
}

var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func text(v string, max int) bool { return len(strings.TrimSpace(v)) > 0 && len(v) <= max }
func (s *Service) Profile(ctx context.Context, user string, name, locale *string) (User, error) {
	u, e := s.Store.User(ctx, user)
	if e != nil {
		return u, e
	}
	if name != nil {
		u.Name = strings.TrimSpace(*name)
	}
	if locale != nil {
		u.Locale = *locale
	}
	if len(u.Name) > 120 || !validLocale(u.Locale) {
		return u, Fail("validation", 422)
	}
	return s.Store.Profile(ctx, user, u.Name, u.Locale)
}

type OrganizationInput struct {
	Name     *string `json:"name"`
	Country  *string `json:"country"`
	Timezone *string `json:"timezone"`
	Locale   *string `json:"default_locale"`
}

func (s *Service) Organization(ctx context.Context, user, org string) (Organization, error) {
	role, e := s.Access(ctx, user, org, false, false)
	if e != nil {
		return Organization{}, e
	}
	o, e := s.Store.Organization(ctx, org)
	o.Role = role
	return o, e
}
func (s *Service) SaveOrganization(ctx context.Context, user, id string, in OrganizationInput) (Organization, error) {
	o := Organization{ID: ID(), Timezone: "UTC", Locale: "en"}
	var e error
	if id != "" {
		if _, e = s.Access(ctx, user, id, true, true); e != nil {
			return o, e
		}
		o, e = s.Store.Organization(ctx, id)
		if e != nil {
			return o, e
		}
	}
	if in.Name != nil {
		o.Name = strings.TrimSpace(*in.Name)
	}
	if in.Country != nil {
		o.Country = strings.ToUpper(*in.Country)
	}
	if in.Timezone != nil {
		o.Timezone = *in.Timezone
	}
	if in.Locale != nil {
		o.Locale = *in.Locale
	}
	_, e = time.LoadLocation(o.Timezone)
	if !text(o.Name, 160) || !regexp.MustCompile(`^[A-Z]{2}$`).MatchString(o.Country) || !validLocale(o.Locale) || e != nil {
		return o, Fail("validation", 422)
	}
	if id == "" {
		o, e = s.Store.AddOrganization(ctx, user, o)
	} else {
		o, e = s.Store.UpdateOrganization(ctx, o)
	}
	o.Role = "owner"
	return o, e
}

type AssistantInput struct {
	Name           *string `json:"name"`
	Description    *string `json:"description"`
	Locale         *string `json:"default_locale"`
	LanguagePolicy *string `json:"language_policy"`
	Status         *string `json:"status"`
}

func (s *Service) Assistants(ctx context.Context, user, org string) ([]Assistant, error) {
	if _, e := s.Access(ctx, user, org, false, false); e != nil {
		return nil, e
	}
	return s.Store.Assistants(ctx, org)
}
func (s *Service) Assistant(ctx context.Context, user, org, id string) (Assistant, error) {
	if _, e := s.Access(ctx, user, org, false, false); e != nil {
		return Assistant{}, e
	}
	if !uuid.MatchString(id) {
		return Assistant{}, Fail("not_found", 404)
	}
	return s.Store.Assistant(ctx, org, id)
}
func (s *Service) SaveAssistant(ctx context.Context, user, org, id string, in AssistantInput) (Assistant, error) {
	if _, e := s.Access(ctx, user, org, true, false); e != nil {
		return Assistant{}, e
	}
	a := Assistant{ID: ID(), OrganizationID: org, Locale: "en", LanguagePolicy: "visitor", Status: "draft"}
	var e error
	if id != "" {
		a, e = s.Assistant(ctx, user, org, id)
		if e != nil {
			return a, e
		}
	}
	if in.Name != nil {
		a.Name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		a.Description = *in.Description
	}
	if in.Locale != nil {
		a.Locale = *in.Locale
	}
	if in.LanguagePolicy != nil {
		a.LanguagePolicy = *in.LanguagePolicy
	}
	if in.Status != nil {
		a.Status = *in.Status
	}
	if a.Status == "active" {
		return a, Fail("ai_not_configured", 422)
	}
	if !text(a.Name, 160) || len(a.Description) > 4000 || !validLocale(a.Locale) || (a.LanguagePolicy != "visitor" && a.LanguagePolicy != "fixed") || a.Status != "draft" {
		return a, Fail("validation", 422)
	}
	return s.Store.SaveAssistant(ctx, a, id == "")
}
func NormalizeDomain(v string) (string, error) {
	v = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(v)), ".")
	ascii, e := idna.Lookup.ToASCII(v)
	if e != nil || len(ascii) > 253 || !strings.Contains(ascii, ".") {
		return "", Fail("validation", 422)
	}
	label := regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	for _, p := range strings.Split(ascii, ".") {
		if !label.MatchString(p) {
			return "", Fail("validation", 422)
		}
	}
	if regexp.MustCompile(`^[0-9.]+$`).MatchString(ascii) {
		return "", Fail("validation", 422)
	}
	return ascii, nil
}

type SiteInput struct {
	Name        string `json:"name"`
	Domain      string `json:"domain"`
	AssistantID string `json:"assistant_id"`
}

func (s *Service) Sites(ctx context.Context, user, org string) ([]Site, error) {
	if _, e := s.Access(ctx, user, org, false, false); e != nil {
		return nil, e
	}
	return s.Store.Sites(ctx, org)
}
func (s *Service) AddSite(ctx context.Context, user, org string, in SiteInput) (Site, error) {
	if _, e := s.Access(ctx, user, org, true, false); e != nil {
		return Site{}, e
	}
	if !text(in.Name, 160) {
		return Site{}, Fail("validation", 422)
	}
	domain, e := NormalizeDomain(in.Domain)
	if e != nil {
		return Site{}, e
	}
	if _, e = s.Assistant(ctx, user, org, in.AssistantID); e != nil {
		return Site{}, e
	}
	return s.Store.AddSite(ctx, Site{ID: ID(), OrganizationID: org, AssistantID: in.AssistantID, Name: strings.TrimSpace(in.Name), Domain: domain})
}
func (s *Service) DeleteSite(ctx context.Context, user, org, id string) error {
	if _, e := s.Access(ctx, user, org, true, false); e != nil {
		return e
	}
	if !uuid.MatchString(id) {
		return Fail("not_found", 404)
	}
	return s.Store.DeleteSite(ctx, org, id)
}
