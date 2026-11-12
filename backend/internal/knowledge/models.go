package knowledge

import "time"

type Source struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	AssistantID    string    `json:"assistant_id"`
	Kind           string    `json:"kind"`
	Title          string    `json:"title"`
	Language       string    `json:"language"`
	URL            string    `json:"url"`
	FileKey        string    `json:"-"`
	Format         string    `json:"format"`
	Enabled        bool      `json:"enabled"`
	Content        string    `json:"content"`
	Checksum       string    `json:"checksum"`
	Crawl          bool      `json:"crawl"`
	ImportedPages  int       `json:"imported_pages"`
	Revision       int       `json:"revision"`
	ActiveRevision *int      `json:"active_revision"`
	Status         string    `json:"status"`
	ErrorCode      string    `json:"error_code"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
type Page struct {
	Text   string `json:"text"`
	Number int    `json:"number"`
}
type Chunk struct {
	Text     string
	Page     int
	Position int
}
type Job struct {
	ID, OrganizationID, AssistantID, SourceID, Token string
	Revision, Attempts                               int
}
type Input struct {
	Title    *string `json:"title"`
	Content  *string `json:"content"`
	Language *string `json:"language"`
	Enabled  *bool   `json:"enabled"`
}
type Hit struct {
	ID           string  `json:"chunk_id"`
	SourceID     string  `json:"document_id"`
	Title        string  `json:"title"`
	Content      string  `json:"content"`
	URL          string  `json:"url"`
	Page         *int    `json:"page"`
	Score        float64 `json:"score"`
	Similarity   float64 `json:"similarity"`
	Revision     int     `json:"index_version"`
	Model        string  `json:"model"`
	ModelVersion string  `json:"model_version"`
	Language     string  `json:"language"`
}
type SearchInput struct {
	Query string `json:"query"`
	Mode  string `json:"mode"`
}
