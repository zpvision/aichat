package ai

import (
	"context"
	"encoding/json"
	"site-assistant/internal/knowledge"
	"time"
)

type Settings struct {
	DisplayName        string `json:"display_name"`
	Role               string `json:"role"`
	CompanyDescription string `json:"company_description"`
	Style              string `json:"style"`
	Instructions       string `json:"instructions"`
	NoAnswer           string `json:"no_answer"`
	LanguagePolicy     string `json:"language_policy"`
	ResponseLocale     string `json:"response_locale"`
	OfferOperator      bool   `json:"offer_operator"`
}
type SettingsPatch struct {
	DisplayName        *string `json:"display_name"`
	Role               *string `json:"role"`
	CompanyDescription *string `json:"company_description"`
	Style              *string `json:"style"`
	Instructions       *string `json:"instructions"`
	NoAnswer           *string `json:"no_answer"`
	LanguagePolicy     *string `json:"language_policy"`
	ResponseLocale     *string `json:"response_locale"`
	OfferOperator      *bool   `json:"offer_operator"`
}
type SettingsResponse struct {
	Settings        Settings `json:"settings"`
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	Demo            bool     `json:"demo"`
	MaxMessageChars int      `json:"max_message_chars"`
}
type Scope struct{ OrganizationID, AssistantID, ConversationID string }
type Conversation struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	AssistantID    string    `json:"assistant_id"`
	Kind           string    `json:"kind"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
type Message struct {
	AuthorUserID *string   `json:"author_user_id"`
	RequestID    string    `json:"request_id"`
	ChangeSeq    int64     `json:"change_seq"`
	ID           string    `json:"id"`
	ExecutionID  string    `json:"execution_id"`
	Sequence     int64     `json:"sequence"`
	Role         string    `json:"role"`
	Content      string    `json:"content"`
	Language     string    `json:"language"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}
type Execution struct {
	ID                 string          `json:"id"`
	RequestID          string          `json:"request_id"`
	Provider           string          `json:"provider"`
	Model              string          `json:"model"`
	Status             string          `json:"status"`
	DurationMS         int64           `json:"duration_ms"`
	InputTokens        *int64          `json:"input_tokens"`
	OutputTokens       *int64          `json:"output_tokens"`
	ErrorCode          string          `json:"error_code"`
	Sources            []knowledge.Hit `json:"sources"`
	ResponseLanguage   string          `json:"response_language"`
	PolicyFallback     bool            `json:"policy_fallback"`
	ProviderDurationNS *int64          `json:"provider_duration_ns"`
	CreatedAt          time.Time       `json:"created_at"`
}
type Detail struct {
	Conversation Conversation `json:"conversation"`
	Messages     []Message    `json:"messages"`
	Executions   []Execution  `json:"executions"`
	NextBefore   *int64       `json:"next_before"`
}
type MessageInput struct {
	RequestID string `json:"request_id"`
	Content   string `json:"content"`
}
type Result struct {
	Manual           bool      `json:"manual"`
	Execution        Execution `json:"execution"`
	UserMessage      Message   `json:"user_message"`
	AssistantMessage Message   `json:"assistant_message"`
	Replay           bool      `json:"replay"`
}
type Event struct {
	Type string
	Data any
}
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type GenerateRequest struct {
	Messages     []ChatMessage
	Language     string
	Fallback     string
	HasKnowledge bool
	JSONSchema   json.RawMessage
}
type Usage struct {
	InputTokens  *int64
	OutputTokens *int64
	DurationNS   *int64
}
type GenerateResult struct {
	Content string
	Usage   Usage
}
type AIProvider interface {
	Generate(ctx context.Context, request GenerateRequest, stream bool, emit func(string) error) (GenerateResult, error)
}
