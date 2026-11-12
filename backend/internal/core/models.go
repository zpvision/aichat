package core

import "time"

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Locale    string    `json:"preferred_locale"`
	CreatedAt time.Time `json:"created_at"`
}
type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Country   string    `json:"country"`
	Timezone  string    `json:"timezone"`
	Locale    string    `json:"default_locale"`
	Role      string    `json:"role,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type Assistant struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Locale         string    `json:"default_locale"`
	LanguagePolicy string    `json:"language_policy"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
type Site struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	AssistantID    string    `json:"assistant_id"`
	Name           string    `json:"name"`
	Domain         string    `json:"domain"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}
type Fault struct {
	Code   string
	Status int
}

func (e *Fault) Error() string           { return e.Code }
func Fail(code string, status int) error { return &Fault{code, status} }
